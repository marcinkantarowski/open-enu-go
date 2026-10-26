package httpx

import _ "embed"

// SharedComponents is the part of the OpenAPI contract the kernel owns: the
// error envelope, the 409 details and the security schemes. tools/genspec
// merges it into backend/openapi.json, and tools/genapi merges it into each
// module's fragment before generating that module's server, so a fragment
// can `$ref: '#/components/schemas/Error'` without carrying a copy.
//
//go:embed components.json
var SharedComponents []byte
