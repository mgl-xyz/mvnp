package npm

import "errors"

var (
	ErrPackageNotFound = errors.New("package not found on registry")
	ErrRateLimited     = errors.New("registry rate limit exceeded")
)
