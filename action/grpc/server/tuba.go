package server

import (
	"context"
	"github.com/hcloud-classic/hcc_errors"
	"github.com/hcloud-classic/pb"
	"hcc/tuba/action/grpc/errconv"
	"hcc/tuba/dao"
)

type tubaServer struct {
	pb.UnimplementedTubaServer
}

func (s *tubaServer) GetTaskList(_ context.Context, _ *pb.Empty) (*pb.ResGetTaskList, error) {
	resGetTaskList, errCode, errStr := dao.ReadTaskList()
	if errCode != 0 {
		errStack := hcc_errors.NewHccErrorStack(hcc_errors.NewHccError(errCode, errStr))
		return &pb.ResGetTaskList{
			Result:        "",
			HccErrorStack: errconv.HccStackToGrpc(errStack),
		}, nil
	}

	return resGetTaskList, nil
}
