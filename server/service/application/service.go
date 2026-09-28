package application

const (
	StableURL       = "https://cdn.sipeed.com/nanokvm"
	PreviewURL      = "https://cdn.sipeed.com/nanokvm/preview"
	forkReleasesURL = "https://api.github.com/repos/pekishev/NanoKVM-Pro/releases?per_page=30"
	updateUserAgent = "NanoKVM-Pro"

	AppDir  = "/kvmapp"
	TempDir = "/root/.kvmcache"
)

var appNames = []string{"nanokvmpro", "pikvm", "kvmcomm"}

type Service struct{}

func NewService() *Service {
	return &Service{}
}
