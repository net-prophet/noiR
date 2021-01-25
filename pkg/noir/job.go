package noir

import (
	pb "github.com/net-prophet/noir/pkg/proto"
	log "github.com/pion/ion-log"
	"github.com/pion/webrtc/v3"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type Job struct {
	id      string
	manager *Manager
	jobData *pb.JobData
}

type PeerJob struct {
	Job
	roomID string
	peerJobData *pb.PeerJobData
	pc *webrtc.PeerConnection
}

type RunnableJob interface {
	Handle()
}

func NewBaseJob(manager *Manager, handler string, jobID string) *Job {
	return &Job{
		id:      jobID,
		manager: manager,
		jobData:    &pb.JobData{
			Id:              jobID,
			Handler:         handler,
			Status:          0,
			Created:         timestamppb.Now(),
			LastUpdate:      timestamppb.Now(),
			NodeID:          manager.ID(),
		},
	}
}

func NewPeerJob(manager *Manager, handler string, roomID string, peerID string) *PeerJob {
	return &PeerJob{
		Job: *NewBaseJob(manager, handler, "job-"+peerID),
		peerJobData: &pb.PeerJobData{
			RoomID:          roomID,
			PeerID:          peerID,
			PublishTracks:   []string{},
			SubscribeTracks: []string{},
		},
	}
}


func (j *Job) GetCommandQueue() Queue {
	return j.manager.GetQueue(pb.KeyTopicFromJob(j.id))
}

func (j *PeerJob) GetPeerQueue() Queue {
	return j.manager.GetQueue(pb.KeyTopicFromPeer(j.peerJobData.GetPeerID()))
}

func (j *Job) GetManager() *Manager {
	return j.manager
}

func (j *Job) GetData() *pb.JobData {
	return j.jobData
}

func (j *Job) Kill(code int) {
	log.Infof("exited %s handler=%s jobid=%s ", code, j.jobData.GetHandler(), j.id)
}

func (j *Job) KillWithError(err error) {
	log.Errorf("job error: %s", err)
	j.Kill(1)
}

func (j *PeerJob) GetPeerData() *pb.PeerJobData {
	return j.peerJobData
}
func (j *PeerJob) GetPeerConnection() *webrtc.PeerConnection {
	return j.pc
}

func (j *PeerJob) GetFromPeerQueue() Queue {
	return j.manager.GetQueue(pb.KeyTopicFromPeer(j.peerJobData.PeerID))
}
