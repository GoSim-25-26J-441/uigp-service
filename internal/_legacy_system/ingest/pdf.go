package ingest

func ParsePDF(path string) (ParsedFile, error) {
	return ParsedFile{
		Name:  path,
		Notes: []string{"pdf: parser stub — vector text/lines not implemented yet; OCR fallback will be added"},
	}, nil
}
