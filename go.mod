module hcc/tuba

go 1.16

require (
	github.com/Terry-Mao/goconf v0.0.0-20161115082538-13cb73d70c44
	google.golang.org/grpc v1.39.0
	innogrid.com/hcloud-classic/hcc_errors v0.0.0
	innogrid.com/hcloud-classic/pb v0.0.0
)

replace (
	innogrid.com/hcloud-classic/hcc_errors => ../hcc_errors
	innogrid.com/hcloud-classic/pb => ../pb
)
