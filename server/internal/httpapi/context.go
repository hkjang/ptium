package httpapi

import (
	"context"

	"github.com/hkjang/ptium/server/internal/model"
)

type requestIDKey struct{}
type userKey struct{}

func withRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, requestIDKey{}, requestID)
}

func RequestID(ctx context.Context) string {
	value, _ := ctx.Value(requestIDKey{}).(string)
	return value
}

func withUser(ctx context.Context, user model.User) context.Context {
	return context.WithValue(ctx, userKey{}, user)
}

func UserFromContext(ctx context.Context) (model.User, bool) {
	user, ok := ctx.Value(userKey{}).(model.User)
	return user, ok && user.ID != ""
}
