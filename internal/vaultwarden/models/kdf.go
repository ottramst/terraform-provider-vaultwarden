package models

// KdfType represents the type of key derivation function used
type KdfType int

const (
	KdfTypePBKDF2_SHA256 KdfType = 0
	KdfTypeArgon2        KdfType = 1
)

// KdfConfiguration represents the key derivation function configuration
type KdfConfiguration struct {
	KdfIterations  int     `json:"kdfIterations,omitempty"`
	KdfMemory      int     `json:"kdfMemory,omitempty"`
	KdfParallelism int     `json:"kdfParallelism,omitempty"`
	KdfType        KdfType `json:"kdfType,omitempty"`
}
