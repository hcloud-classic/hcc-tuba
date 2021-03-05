module hcc/tuba

go 1.16

require (
	github.com/Terry-Mao/goconf v0.0.0-20161115082538-13cb73d70c44
	github.com/hcloud-classic/hcc_errors v1.1.2
	github.com/hcloud-classic/pb v0.0.0
	github.com/mitchellh/go-ps v1.0.0
	google.golang.org/grpc v1.34.1
)

replace github.com/hcloud-classic/pb => ../pb
