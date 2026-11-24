package imagemodel

type ReferenceImage struct {
	Data     []byte
	MIMEType string
	FileName string
}
type GenerateRequest struct {
	Prompt          string
	ReferenceImages []ReferenceImage
	Mask            *ReferenceImage
	Size            string
	AspectRatio     string
	Background      string
	Quality         string
	OutputFormat    string
}
type GenerateResult struct {
	Data        []byte
	MIMEType    string
	RevisedText string
}
