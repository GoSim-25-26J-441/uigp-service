package ingest

// Placeholder: later we’ll do OpenCV + Tesseract here (SketchMicro tokens + arrows).
func ParseRaster(path string) (ParsedFile, error) {
	return ParsedFile{
		Name:  path,
		Notes: []string{"raster: stub — OpenCV+Tesseract pipeline pending"},
	}, nil
}
