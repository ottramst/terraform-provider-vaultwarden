package models

type User struct {
	ID            string         `json:"id"`
	Email         string         `json:"email"`
	Key           string         `json:"key"`
	PrivateKey    string         `json:"privateKey"`
	Organizations []Organization `json:"organizations,omitempty"`
}
