package files

import "context"

type Service interface {
	Upload(context.Context, UploadRequest) (Asset, error)
	Share(context.Context, ShareRequest) (Asset, error)
	Info(context.Context, string) (Asset, error)
	Download(context.Context, DownloadRequest) (Download, error)
}
