package server

import (
	"context"
	"hcc/tuba/action/grpc/errconv"
	"hcc/tuba/dao"
	"innogrid.com/hcloud-classic/hcc_errors"
	"innogrid.com/hcloud-classic/pb"
	"sync"
)

type tubaServer struct {
	pb.UnimplementedTubaServer
}

var getTaskListLock sync.Mutex

func (s *tubaServer) GetTaskList(_ context.Context, reqGetTaskList *pb.ReqGetTaskList) (*pb.ResGetTaskList, error) {
	getTaskListLock.Lock()

	resGetTaskList, errCode, errStr := dao.ReadTaskList(reqGetTaskList)
	if errCode != 0 {
		errStack := hcc_errors.NewHccErrorStack(hcc_errors.NewHccError(errCode, errStr))

		getTaskListLock.Unlock()
		return &pb.ResGetTaskList{
			Result:        []byte{},
			HccErrorStack: errconv.HccStackToGrpc(errStack),
		}, nil
	}

	getTaskListLock.Unlock()
	return resGetTaskList, nil
}
