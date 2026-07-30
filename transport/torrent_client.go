package transport

// TorrentClient — абстракция над торрент-клиентом (qBittorrent, Deluge, …).
type TorrentClient interface {
	Name() string
	AddMagnet(magnetURI string, savePath string) (hash string, err error)
	AddTorrentFile(data []byte, savePath string) (hash string, err error)
	GetInfo(hash string) (TorrentInfo, error)
	List() ([]TorrentInfo, error)
	Delete(hash string, deleteFiles bool) error
	Test() error
}

// TorrentInfo — статус торрента.
type TorrentInfo struct {
	Hash     string
	Name     string
	Progress float64 // 0.0 – 1.0
	State    string  // "downloading" | "seeding" | "paused" | "error"
	SavePath string
	Size     int64
}
