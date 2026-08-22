package generation

import (
	"context"
	"errors"
	"strings"
)

// AuthorMessage is what the person who asked for the deck is told when the
// generation fails.
//
// The technical cause goes to the incident record, where an operator reads it.
// It used to go to the author as well, which put this on their screen:
//
//	AI request failed: Post "http://10.0.4.19:11300/v1/chat/completions":
//	dial tcp 10.0.4.19:11300: connect: connection refused
//
// That tells the author nothing they can act on, and tells anyone with an
// account the address and port of an internal service. What they need is which
// kind of thing went wrong, whether trying again is worth it, and who to ask.
func AuthorMessage(cause error, language string) string {
	words := failureWordsFor(language)
	if cause == nil {
		return words.unknown
	}
	var rejected rejectedRequest
	if errors.As(cause, &rejected) {
		switch {
		case rejected.status == 401 || rejected.status == 403:
			return words.rejected
		case rejected.status == 429:
			return words.busy
		case rejected.status >= 500:
			return words.providerBroken
		}
		return words.providerBroken
	}
	if errors.Is(cause, context.DeadlineExceeded) || errors.Is(cause, context.Canceled) {
		return words.timeout
	}
	text := strings.ToLower(cause.Error())
	switch {
	case strings.Contains(text, "connection refused"), strings.Contains(text, "no such host"),
		strings.Contains(text, "dial tcp"), strings.Contains(text, "network is unreachable"),
		strings.Contains(text, "base url is invalid"), strings.Contains(text, "connect:"):
		return words.unreachable
	case strings.Contains(text, "timeout"), strings.Contains(text, "deadline exceeded"):
		return words.timeout
	case strings.Contains(text, "invalid json"), strings.Contains(text, "no choices"),
		strings.Contains(text, "could not be read"), strings.Contains(text, "empty outline"),
		strings.Contains(text, "without slides"), strings.Contains(text, "reasoning but no answer"):
		return words.unreadable
	case strings.Contains(text, "template"):
		return words.template
	case strings.Contains(text, "needs an ai provider"), strings.Contains(text, "unsupported ai provider"):
		return words.notConfigured
	}
	return words.unknown
}

// failureWords is what each kind of failure is called, in the language the deck
// was asked for.
type failureWords struct {
	unreachable, rejected, busy, timeout, providerBroken, unreadable, template, notConfigured, unknown string
}

func failureWordsKorean() failureWords {
	return failureWords{
		unreachable:    "AI 서비스에 연결하지 못했습니다. 관리자에게 서비스 설정의 모델 연결을 확인해 달라고 요청하세요.",
		rejected:       "AI 서비스가 요청을 거부했습니다(인증). 관리자에게 API 키를 확인해 달라고 요청하세요.",
		busy:           "AI 서비스가 지금 요청을 더 받지 못하고 있습니다. 잠시 뒤 다시 시도해 주세요.",
		timeout:        "AI 서비스가 제한 시간 안에 답하지 않았습니다. 다시 시도해 주세요. 계속되면 관리자에게 알려 주세요.",
		providerBroken: "AI 서비스가 오류를 돌려주었습니다. 다시 시도해 주세요. 계속되면 관리자에게 오류 센터를 확인해 달라고 요청하세요.",
		unreadable:     "AI 서비스가 읽을 수 없는 답을 보냈습니다. 다시 시도해 주세요. 계속되면 관리자에게 모델 설정을 확인해 달라고 요청하세요.",
		template:       "이 덱의 템플릿을 불러오지 못했습니다. 다른 디자인을 고르거나 관리자에게 알려 주세요.",
		notConfigured:  "AI 제공자가 설정되어 있지 않습니다. 관리자에게 서비스 설정을 확인해 달라고 요청하세요.",
		unknown:        "생성에 실패했습니다. 다시 시도해 주세요. 계속되면 관리자에게 오류 센터의 기록을 확인해 달라고 요청하세요.",
	}
}

func failureWordsEnglish() failureWords {
	return failureWords{
		unreachable:    "The AI service could not be reached. Ask an administrator to check the model connection in service settings.",
		rejected:       "The AI service refused the request (authentication). Ask an administrator to check the API key.",
		busy:           "The AI service is not taking more requests right now. Please try again shortly.",
		timeout:        "The AI service did not answer in time. Please try again; tell an administrator if it keeps happening.",
		providerBroken: "The AI service returned an error. Please try again; if it keeps happening, ask an administrator to check the error centre.",
		unreadable:     "The AI service returned an answer that could not be read. Please try again; if it keeps happening, ask an administrator to check the model settings.",
		template:       "This deck's template could not be loaded. Choose another design, or tell an administrator.",
		notConfigured:  "No AI provider is configured. Ask an administrator to check service settings.",
		unknown:        "Generation failed. Please try again; if it keeps happening, ask an administrator to check the record in the error centre.",
	}
}

func failureWordsJapanese() failureWords {
	return failureWords{
		unreachable:    "AIサービスに接続できませんでした。管理者にサービス設定のモデル接続をご確認ください。",
		rejected:       "AIサービスがリクエストを拒否しました（認証）。管理者にAPIキーをご確認ください。",
		busy:           "AIサービスが現在これ以上のリクエストを受け付けていません。しばらくしてからお試しください。",
		timeout:        "AIサービスが制限時間内に応答しませんでした。再度お試しください。続く場合は管理者にお知らせください。",
		providerBroken: "AIサービスがエラーを返しました。再度お試しください。続く場合は管理者にエラーセンターの確認を依頼してください。",
		unreadable:     "AIサービスが読み取れない応答を返しました。再度お試しください。続く場合は管理者にモデル設定の確認を依頼してください。",
		template:       "このデッキのテンプレートを読み込めませんでした。別のデザインを選ぶか、管理者にお知らせください。",
		notConfigured:  "AIプロバイダーが設定されていません。管理者にサービス設定の確認を依頼してください。",
		unknown:        "生成に失敗しました。再度お試しください。続く場合は管理者にエラーセンターの記録の確認を依頼してください。",
	}
}

func failureWordsChinese() failureWords {
	return failureWords{
		unreachable:    "无法连接到 AI 服务。请联系管理员检查服务设置中的模型连接。",
		rejected:       "AI 服务拒绝了请求（认证）。请联系管理员检查 API 密钥。",
		busy:           "AI 服务目前无法接受更多请求，请稍后再试。",
		timeout:        "AI 服务未在限定时间内响应。请重试；如持续出现请告知管理员。",
		providerBroken: "AI 服务返回了错误。请重试；如持续出现，请联系管理员查看错误中心。",
		unreadable:     "AI 服务返回了无法读取的内容。请重试；如持续出现，请联系管理员检查模型设置。",
		template:       "无法加载该演示文稿的模板。请更换设计，或告知管理员。",
		notConfigured:  "尚未配置 AI 提供方。请联系管理员检查服务设置。",
		unknown:        "生成失败。请重试；如持续出现，请联系管理员查看错误中心的记录。",
	}
}

func failureWordsFor(language string) failureWords {
	switch {
	case strings.HasPrefix(strings.ToLower(language), "ja"):
		return failureWordsJapanese()
	case strings.HasPrefix(strings.ToLower(language), "zh"):
		return failureWordsChinese()
	case strings.HasPrefix(strings.ToLower(language), "ko"), strings.TrimSpace(language) == "":
		return failureWordsKorean()
	}
	return failureWordsEnglish()
}
