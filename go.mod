module hcc/tuba

go 1.16

require (
	github.com/Terry-Mao/goconf v0.0.0-20161115082538-13cb73d70c44
	github.com/cespare/xxhash v1.1.0 // indirect
	google.golang.org/grpc v1.41.0
	innogrid.com/hcloud-classic/hcc_errors v0.0.0
	innogrid.com/hcloud-classic/pb v0.0.0
)

replace (
	innogrid.com/hcloud-classic/hcc_errors => ../hcc_errors
	innogrid.com/hcloud-classic/pb => ../pb
)
