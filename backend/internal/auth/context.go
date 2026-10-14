package auth

import "context"

// User is the signed-in caller, as the auth middleware found them.
type User struct {
	ID      int64
	IsAdmin bool
}

// ctxKey is the private key type for values this package stores in a
// context, so no other package can collide with or forge them.
type ctxKey struct{}

// WithUser returns a copy of ctx carrying the signed-in user.
func WithUser(ctx context.Context, u User) context.Context {
	return context.WithValue(ctx, ctxKey{}, u)
}

// UserFrom returns the user stored by WithUser, or false when the request
// did not pass the auth middleware.
func UserFrom(ctx context.Context) (User, bool) {
	u, ok := ctx.Value(ctxKey{}).(User)
	return u, ok
}
