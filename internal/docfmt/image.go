package docfmt

// sizeFromTitle reads an image's size from the title Markdown allows after its
// address, `![alt](src "large")`: a title that is not a size name is not one.
func sizeFromTitle(title []byte) string {
	if _, ok := ImageSizes[string(title)]; ok {
		return string(title)
	}
	return ""
}
