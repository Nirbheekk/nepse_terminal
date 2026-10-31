// Package contracts embeds the OpenAPI document and the service version so a
// running binary can serve its own specification without touching disk.
//
// openapi.yaml here is a generated copy of ../../../../contracts/openapi.yaml,
// which is the canonical contract; scripts/sync-contracts.sh refreshes it.
package contracts

import _ "embed"

// Version is reported by /healthz and the OpenAPI document.
const Version = "0.1.0"

//go:embed openapi.yaml
var Spec []byte
