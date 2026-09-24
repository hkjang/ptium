package auth

// Refusal is an ErrInvalidCredentials that also carries a sentence safe to
// show the client.
//
// Most authentication failures rightly reach the client as "Unauthorized" and
// nothing more. An MCP client refused with an SSO token is the exception: the
// person reading the answer is the operator wiring Keycloak, and what they
// need is which claim was wrong and what to put where. The cause — the exact
// check that failed — goes to the log through Error; Message is what the
// client is told.
type Refusal struct {
	Message string
	Cause   error
}

func (refusal *Refusal) Error() string {
	if refusal.Cause == nil {
		return refusal.Message
	}
	return refusal.Cause.Error()
}

// Unwrap makes every Refusal an ErrInvalidCredentials to errors.Is, and keeps
// the cause reachable.
func (refusal *Refusal) Unwrap() []error {
	if refusal.Cause == nil {
		return []error{ErrInvalidCredentials}
	}
	return []error{ErrInvalidCredentials, refusal.Cause}
}
