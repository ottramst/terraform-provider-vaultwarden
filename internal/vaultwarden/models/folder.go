package models

// Folder is a folder in the personal vault of a user.
type Folder struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	RevisionDate string `json:"revisionDate"`
	Object       string `json:"object"`
}

// Folders is the response of the folder list endpoint.
type Folders struct {
	Data []Folder `json:"data"`
}
