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

func (s *tubaServer) GetProcessList(_ context.Context, in *pb.ReqGetProcessList) (*pb.ResGetProcessList, error) {
	resGetProcessList, errCode, errStr := dao.ReadProcessList(in)
	if errCode != 0 {
		errStack := hcc_errors.NewHccErrorStack(hcc_errors.NewHccError(errCode, errStr))
		return &pb.ResGetProcessList{Process: []*pb.Process{}, HccErrorStack: errconv.HccStackToGrpc(errStack)}, nil
	}

	return &pb.ResGetProcessList{Process: resGetProcessList.Process}, nil
}
