package types

type Attachment struct {
	Name        string `json:"name"`
	ContentType string `json:"content_type"`
	DataBase64  string `json:"data_base64"`
}
