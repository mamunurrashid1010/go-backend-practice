// context.Value — request-scoped values.
//
// The type-key pattern uses an unexported struct type as the key, so
// no other package can construct a colliding key even if they know
// the name.
//
// Run: go run ./cmd/values
package main

import (
	"context"
	"fmt"
)

// package "request" — provides RequestID plumbing
type requestIDKey struct{} // unexported: cannot be constructed outside this package

func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey{}, id)
}
func GetRequestID(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(requestIDKey{}).(string)
	return id, ok
}

// package "auth" — provides UserID plumbing. Same NAME "requestIDKey"
// wouldn't matter because the type is unexported and distinct.
type userIDKey struct{}

func WithUserID(ctx context.Context, id int64) context.Context {
	return context.WithValue(ctx, userIDKey{}, id)
}
func GetUserID(ctx context.Context) (int64, bool) {
	id, ok := ctx.Value(userIDKey{}).(int64)
	return id, ok
}

// Deep in the call tree: reads whatever was attached upstream.
func handler(ctx context.Context) {
	if rid, ok := GetRequestID(ctx); ok {
		fmt.Printf("  handler: rid=%s\n", rid)
	}
	if uid, ok := GetUserID(ctx); ok {
		fmt.Printf("  handler: uid=%d\n", uid)
	}
}

func main() {
	fmt.Println("=== attach two values to the same ctx ===")
	ctx := context.Background()
	ctx = WithRequestID(ctx, "abc123")
	ctx = WithUserID(ctx, 42)

	// The values live on the ctx tree; every child sees them.
	handler(ctx)

	fmt.Println("\n=== attaching to a child doesn't pollute the parent ===")
	base := context.Background()
	child := WithRequestID(base, "req-1")
	rid, ok := GetRequestID(base)
	fmt.Printf("  base:  rid=%q ok=%v\n", rid, ok)
	rid, ok = GetRequestID(child)
	fmt.Printf("  child: rid=%q ok=%v\n", rid, ok)

	// The "no" cases from the README:
	//   * Passing a *sql.DB via ctx.Value — do NOT. Pass as a param.
	//   * Passing a logger — YES, it's request-scoped (see the notes
	//     API's `logging.With(ctx, l)` since Day 25).
}
