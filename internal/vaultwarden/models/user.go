package models

type User struct {
	ID            string         `json:"id"`
	Name          string         `json:"name"`
	Email         string         `json:"email"`
	Key           string         `json:"key"`
	PrivateKey    string         `json:"privateKey"`
	Organizations []Organization `json:"organizations,omitempty"`
}
