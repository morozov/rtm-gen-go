// Package fetch retrieves the RTM reflection spec live via the
// generated client. The live code path requires the `livefetch`
// build tag; without it, Fetch is a stub that returns
// ErrLiveFetchDisabled.
package fetch
