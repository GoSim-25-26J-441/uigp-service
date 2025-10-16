package ingest

// Placeholder: we’ll add pdf text extraction (vector) and OCR fallback later.
func ParsePDF(path string) (ParsedFile, error) {
	return ParsedFile{
		Name:  path,
		Notes: []string{"pdf: parser stub — vector text/lines not implemented yet; OCR fallback will be added"},
	}, nil
}
