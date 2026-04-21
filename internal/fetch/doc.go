// Package fetch retrieves the RTM reflection spec via live HTTP
// calls. It issues the two reflection methods directly through
// stdlib net/http with inline MD5 signing; the package has no
// dependency on any generated artifact.
package fetch
