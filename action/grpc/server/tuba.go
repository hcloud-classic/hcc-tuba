package server

import (
	"context"
	"hcc/tuba/action/grpc/errconv"
	"hcc/tuba/dao"
	"innogrid.com/hcloud-classic/hcc_errors"
	"innogrid.com/hcloud-classic/pb"
)

type tubaServer struct {
	pb.UnimplementedTubaServer
}

func (s *tubaServer) GetTaskList(_ context.Context, reqGetTaskList *pb.ReqGetTaskList) (*pb.ResGetTaskList, error) {
	resGetTaskList, errCode, errStr := dao.ReadTaskList(reqGetTaskList)
	if errCode != 0 {
		errStack := hcc_errors.NewHccErrorStack(hcc_errors.NewHccError(errCode, errStr))
		return &pb.ResGetTaskList{
			Result:        []byte{},
			HccErrorStack: errconv.HccStackToGrpc(errStack),
		}, nil
	}

	return resGetTaskList, nil
}
