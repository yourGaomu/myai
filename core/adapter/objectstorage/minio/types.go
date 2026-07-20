package minioadapter

type uploadInfo struct {
	Size int64
}

type storedObjectInfo struct {
	ObjectKey   string
	Size        int64
	ContentType string
	Metadata    map[string]string
}
