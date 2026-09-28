"""Silent sign-in and sign-out against a real Keycloak, in a real browser.

The code for silent SSO had been in place for a while and its unit tests
passed. Driven end to end against Keycloak 26 it signed people in correctly —
and signing out was broken twice: Keycloak stopped on its own "Do you want to
log out?" page because no id_token_hint was sent, and a new tab opened after
signing out was signed straight back in because "signed out" was kept in the
tab's own storage. Neither can be seen without a provider, so this sweep
brings one.

It needs a Keycloak it may configure (it creates the realm, client and user it
uses) and a Ptium pointed at that realm:

    docker run -d --name ptium-kc -p 18180:8080 \\
      -e KC_BOOTSTRAP_ADMIN_USERNAME=admin -e KC_BOOTSTRAP_ADMIN_PASSWORD=admin \\
      quay.io/keycloak/keycloak:26.4 start-dev
    OIDC_ISSUER_URL=http://localhost:18180/realms/ptium OIDC_CLIENT_ID=ptium \\
      OIDC_ALLOW_HTTP=true ... ptium            # the usual dev settings besides
    PTIUM_URL=http://localhost:8097 KEYCLOAK_URL=http://localhost:18180 \\
      python3 scripts/e2e/sso.py

Turning auth.oidc.auto_login on is part of the sweep; it is put back as found.
"""
import json
import os
import sys
import urllib.error
import urllib.parse
import urllib.request

from playwright.sync_api import sync_playwright

APP = os.environ.get("PTIUM_URL", "http://localhost:8097").rstrip("/")
KEYCLOAK = os.environ.get("KEYCLOAK_URL", "http://localhost:18180").rstrip("/")
ADMIN = (os.environ.get("KEYCLOAK_ADMIN", "admin"), os.environ.get("KEYCLOAK_ADMIN_PASSWORD", "admin"))
SECRET = os.environ.get("PTIUM_DEV_SECRET", "devsecret-devsecret-devsecret-devsecret")
REALM, CLIENT, USER, PASSWORD = "ptium", "ptium", "hong", "hong1234"
REALM_URL = f"{KEYCLOAK}/realms/{REALM}"
failures, checks = [], 0


def check(name, ok, detail=""):
    global checks
    checks += 1
    print(("✓" if ok else "✗"), name, "·", detail)
    if not ok:
        failures.append(f"{name}: {detail}")


def call(method, url, body=None, headers=None, form=False):
    data = None
    headers = dict(headers or {})
    if body is not None:
        data = urllib.parse.urlencode(body).encode() if form else json.dumps(body).encode()
        headers["Content-Type"] = "application/x-www-form-urlencoded" if form else "application/json"
    request = urllib.request.Request(url, data=data, headers=headers, method=method)
    try:
        with urllib.request.urlopen(request) as response:
            raw = response.read()
            return response.status, json.loads(raw) if raw else None
    except urllib.error.HTTPError as error:
        return error.code, None


def prepare_keycloak():
    """The realm, a public PKCE client for this Ptium, and one person."""
    _, token = call("POST", f"{KEYCLOAK}/realms/master/protocol/openid-connect/token",
                    {"grant_type": "password", "client_id": "admin-cli",
                     "username": ADMIN[0], "password": ADMIN[1]}, form=True)
    admin = {"Authorization": f"Bearer {token['access_token']}"}
    base = f"{KEYCLOAK}/admin/realms"
    call("POST", base, {"realm": REALM, "enabled": True}, admin)
    client = {"clientId": CLIENT, "publicClient": True, "standardFlowEnabled": True,
              "redirectUris": [f"{APP}/auth/callback"], "webOrigins": [APP],
              "attributes": {"pkce.code.challenge.method": "S256", "post.logout.redirect.uris": f"{APP}/*"}}
    status, _ = call("POST", f"{base}/{REALM}/clients", client, admin)
    if status == 409:
        _, found = call("GET", f"{base}/{REALM}/clients?clientId={CLIENT}", headers=admin)
        call("PUT", f"{base}/{REALM}/clients/{found[0]['id']}", client, admin)
    call("POST", f"{base}/{REALM}/users", {
        "username": USER, "email": f"{USER}@example.com", "firstName": "길동", "lastName": "홍",
        "enabled": True, "emailVerified": True,
        "credentials": [{"type": "password", "value": PASSWORD, "temporary": False}]}, admin)


def auto_login(value):
    status, answer = call("PUT", f"{APP}/api/v1/admin/settings/auth.oidc.auto_login", {"value": value},
                          {"X-Ptium-Dev-Secret": SECRET})
    return status


def settled(page, wait=6000):
    try:
        page.wait_for_load_state("networkidle", timeout=wait)
    except Exception:
        pass
    page.wait_for_timeout(800)


def navigations(page):
    # Requests, not committed frames: Keycloak answers prompt=none with a 302,
    # which never commits, so a frame listener sees the round trip as nothing.
    seen = []
    page.on("request", lambda request: request.is_navigation_request() and seen.append(request.url))
    return seen


def keycloak_sign_in(page):
    page.fill("#username", USER)
    page.fill("#password", PASSWORD)
    page.click("#kc-login")
    settled(page)


def main():
    _, published = call("GET", f"{APP}/api/v1/auth/config")
    if not ((published or {}).get("data") or {}).get("oidcEnabled"):
        sys.exit(f"{APP} is not pointed at an identity provider; see the top of this file")
    prepare_keycloak()
    _, before = call("GET", f"{APP}/api/v1/auth/config")
    was = bool(((before or {}).get("data") or {}).get("autoLogin"))
    auto_login(True)
    try:
        run()
    finally:
        auto_login(was)
    print(f"\n{checks} checks, {len(failures)} failures")
    for failure in failures:
        print("  -", failure)
    sys.exit(1 if failures else 0)


def run():
    with sync_playwright() as playwright:
        browser = playwright.chromium.launch()
        context = browser.new_context()

        print("── nobody signed in at the provider ──")
        page = context.new_page()
        seen = navigations(page)
        page.goto(f"{APP}/presentations")
        settled(page)
        asked = sum("prompt=none" in url for url in seen)
        check("the provider is asked once, silently", asked == 1, f"{asked} time(s)")
        check("and the answer lands on the login screen, marked", page.url.endswith("/login?sso=none"), page.url)
        before = len(seen)
        page.reload()
        settled(page)
        check("a reload does not ask again", not any("prompt=none" in url for url in seen[before:]), page.url)
        check("the screen says why", "자동으로 로그인하지 않았습니다" in page.inner_text("body"), "")
        page.close()

        print("── signed in at the provider by something else ──")
        account = context.new_page()
        account.goto(f"{REALM_URL}/account")
        settled(account)
        keycloak_sign_in(account)
        account.close()
        tab = context.new_page()
        seen = navigations(tab)
        tab.goto(f"{APP}/presentations?view=list")
        settled(tab, 10000)
        check("a new tab comes in with no login screen", tab.url == f"{APP}/presentations?view=list", tab.url)
        check("without Keycloak's password page", not any("login-actions" in url for url in seen), "")
        cookies = {cookie["name"]: cookie for cookie in context.cookies()}
        hint = cookies.get("ptium_id_hint")
        check("the ID token is kept where no script can read it",
              bool(hint) and hint["httpOnly"] and hint["path"] == "/api/v1/auth"
              and "ptium_id_hint" not in tab.evaluate("document.cookie"), str(hint and hint["path"]))

        print("── signing out ──")
        seen = navigations(tab)
        tab.goto(f"{APP}/dashboard")
        tab.wait_for_selector("button.account-trigger", timeout=15000)
        tab.click("button.account-trigger")
        tab.click(".account-menu >> text=로그아웃")
        settled(tab, 10000)
        check("back on the login screen, not on Keycloak's confirmation page",
              tab.url == f"{APP}/login" and "Do you want to log out" not in tab.inner_text("body"), tab.url)
        check("the ID token goes with the session",
              "ptium_id_hint" not in {cookie["name"] for cookie in context.cookies()}, "")
        provider = context.new_page()
        provider.goto(f"{REALM_URL}/account")
        settled(provider)
        # The account console has a username field of its own, so the address
        # is what says whether the provider still knows this browser.
        check("the provider's session is over too", "/protocol/openid-connect/auth" in provider.url, provider.url[:80])
        provider.close()
        fresh = context.new_page()
        fresh.goto(f"{APP}/dashboard")
        settled(fresh, 10000)
        check("a new tab after signing out stays signed out", "/login" in fresh.url, fresh.url)

        print("── signing in again ──")
        fresh.goto(f"{APP}/login")
        fresh.click("text=회사 계정으로 SSO 로그인")
        settled(fresh)
        keycloak_sign_in(fresh)
        settled(fresh, 10000)
        check("the organisation button signs in", fresh.url.endswith("/dashboard"), fresh.url)
        again = context.new_page()
        again.goto(f"{APP}/presentations")
        settled(again, 10000)
        check("and silent sign-in works again in a new tab", again.url == f"{APP}/presentations", again.url)
        browser.close()


if __name__ == "__main__":
    main()
