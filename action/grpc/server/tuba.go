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

func (s *tubaServer) GetTaskList(_ context.Context, in *pb.ReqGetTaskList) (*pb.ResGetTaskList, error) {
	resGetTaskList, errCode, errStr := dao.ReadTaskList(in)
	if errCode != 0 {
		errStack := hcc_errors.NewHccErrorStack(hcc_errors.NewHccError(errCode, errStr))
		return &pb.ResGetTaskList{Tasks: []*pb.Task{}, HccErrorStack: errconv.HccStackToGrpc(errStack)}, nil
	}

	return &pb.ResGetTaskList{
		Tasks:                resGetTaskList.Tasks,
		TotalTasks:           resGetTaskList.TotalTasks,
		TotalMemUsage:        resGetTaskList.TotalMemUsage,
		TotalMem:             resGetTaskList.TotalMem,
		TotalMemUsagePercent: resGetTaskList.TotalMemUsagePercent,
		TotalCPUUsage:        resGetTaskList.TotalCPUUsage,
	}, nil
}
