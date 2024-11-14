package models

type KdfType int

const (
	KdfTypePBKDF2_SHA256 KdfType = 0
	KdfTypeArgon2        KdfType = 1
)

type KdfConfiguration struct {
	KdfIterations  int     `json:"kdfIterations,omitempty"`
	KdfMemory      int     `json:"kdfMemory,omitempty"`
	KdfParallelism int     `json:"kdfParallelism,omitempty"`
	KdfType        KdfType `json:"kdfType,omitempty"`
}
