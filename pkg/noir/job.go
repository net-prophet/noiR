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
	roomID      string
	peerJobData *pb.PeerJobData
	pc          *webrtc.PeerConnection
}

type RunnableJob interface {
	Handle()
}

func NewBaseJob(manager *Manager, handler string, jobID string) *Job {
	return &Job{
		id:      jobID,
		manager: manager,
		jobData: &pb.JobData{
			Id:         jobID,
			Handler:    handler,
			Status:     0,
			Created:    timestamppb.Now(),
			LastUpdate: timestamppb.Now(),
			NodeID:     manager.ID(),
		},
	}
}

func NewPeerJob(manager *Manager, handler string, roomID string, jobID string) *PeerJob {
	userID := "job-" + handler + "-" + jobID
	return &PeerJob{
		Job: *NewBaseJob(manager, handler, jobID),
		peerJobData: &pb.PeerJobData{
			RoomID:          roomID,
			UserID:          userID,
			PublishTracks:   []string{},
			SubscribeTracks: []string{},
		},
	}
}

func (j *Job) GetCommandQueue() Queue {
	return j.manager.GetQueue(pb.KeyTopicFromJob(j.id))
}

func (j *PeerJob) GetQueueFromPeer() Queue {
	return j.manager.GetQueue(pb.KeyTopicFromPeer(j.peerJobData.GetUserID()))
}
func (j *PeerJob) GetQueueToPeer() Queue {
	return j.manager.GetQueue(pb.KeyTopicToPeer(j.peerJobData.GetUserID()))
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
func (j *PeerJob) Kill(code int) {
	log.Infof("exited %s handler=%s jobid=%s userid=%s", code, j.jobData.GetHandler(), j.id, j.peerJobData.UserID)
	j.manager.DisconnectUser(j.peerJobData.UserID)
	if j.pc != nil {
		j.pc.Close()
	}
}

func (j *Job) KillWithError(err error) {
	log.Errorf("job error: %s", err)
	j.Kill(1)
}

func (j *PeerJob) KillWithError(err error) {
	log.Errorf("job error: %s", err)
	j.Kill(1)
}

func (j *PeerJob) GetPeerData() *pb.PeerJobData {
	return j.peerJobData
}
func (j *PeerJob) GetPeerConnection() (*webrtc.PeerConnection, error) {
	if j.pc == nil {
		mediaEngine := webrtc.MediaEngine{}
		mediaEngine.RegisterDefaultCodecs()

		// Create a new RTCPeerConnection
		api := webrtc.NewAPI(webrtc.WithMediaEngine(&mediaEngine))
		pc, err := api.NewPeerConnection(webrtc.Configuration{
			ICEServers: []webrtc.ICEServer{
				{
					URLs: []string{"stun:stun.l.google.com:19302"},
				},
			},
		})
		if err != nil {
			log.Errorf("error getting pc %s", err)
			return nil, err
		}
		j.pc = pc
	}
	return j.pc, nil
}

func (j *PeerJob) SendJoin() error {
	router := j.GetManager().GetRouter()
	queue := (*router).GetQueue()
	userID := j.GetPeerData().UserID
	roomID := j.GetPeerData().RoomID
	pc, err := j.GetPeerConnection()
	if err != nil {
		return err
	}

	log.Infof("joining room=%s as=%s", roomID, userID)
	return EnqueueRequest(*queue, &pb.NoirRequest{
		Command: &pb.NoirRequest_Signal{
			Signal: &pb.SignalRequest{
				Id: userID,
				Payload: &pb.SignalRequest_Join{
					Join: &pb.JoinRequest{
						Sid:         roomID,
						Description: []byte(pc.LocalDescription().SDP),
					},
				},
			},
		},
	})
}
