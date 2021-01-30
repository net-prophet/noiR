package noir

import (
	"encoding/json"
	"github.com/golang/protobuf/proto"
	pb "github.com/net-prophet/noir/pkg/proto"
	log "github.com/pion/ion-log"
	"github.com/pion/webrtc/v3"
	"google.golang.org/protobuf/types/known/timestamppb"
	"io"
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
	mediaEngine *webrtc.MediaEngine
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
		Job:         *NewBaseJob(manager, handler, jobID),
		mediaEngine: &webrtc.MediaEngine{},
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

func (j *PeerJob) GetMediaEngine() *webrtc.MediaEngine {
	return j.mediaEngine
}

func (j *PeerJob) GetPeerConnection() (*webrtc.PeerConnection, error) {
	if j.pc == nil {
		// Create a new RTCPeerConnection
		api := webrtc.NewAPI(webrtc.WithMediaEngine(j.mediaEngine))
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

		pc.OnICECandidate(func(c *webrtc.ICECandidate) {
			if c == nil {
				// Gathering done
				return
			}
			bytes, err := json.Marshal(c.ToJSON())
			if err != nil {
				log.Errorf("OnIceCandidate error %s", err)
			}
			log.Debugf("send candidate:\n %s", string(bytes))
			err = EnqueueRequest(j.GetQueueToPeer(),
				&pb.NoirRequest{
					Command: &pb.NoirRequest_Signal{
						Signal: &pb.SignalRequest{
							Id: j.peerJobData.UserID,
							Payload: &pb.SignalRequest_Trickle{
								Trickle: &pb.Trickle{
									Init: string(bytes),
								},
							},
						},
					},
				},
			)
			if err != nil {
				log.Errorf("OnIceCandidate error %s", err)
			}
		})
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

func (j *PeerJob) WaitForReply() (*pb.NoirReply, error) {
	message, err := j.GetQueueFromPeer().BlockUntilNext(QueueMessageTimeout)

	if err == io.EOF {
		return nil, nil
	}

	if err != nil {
		j.KillWithError(err)
		return nil, err
	}

	var reply pb.NoirReply

	err = proto.Unmarshal(message, &reply)
	if err != nil {
		j.KillWithError(err)
		return nil, err
	}

	return &reply, nil
}

func (j *PeerJob) SendSignalRequest(request *pb.SignalRequest) error {
	request.Id = j.peerJobData.UserID

	return EnqueueRequest(j.GetQueueToPeer(), &pb.NoirRequest{
		Command: &pb.NoirRequest_Signal{Signal: request},
	})
}

func (j *PeerJob) PeerBridge() {
	for {
		reply, err := j.WaitForReply()
		if err != nil {
			return
		}
		if reply == nil {
			continue
		}

		if signal, ok := reply.Command.(*pb.NoirReply_Signal); ok {
			if join := signal.Signal.GetJoin(); join != nil {
				log.Debugf("%s joined %s => %s!\n", j.jobData.Handler, signal.Signal.Id)
				// Set the remote SessionDescription
				desc := &webrtc.SessionDescription{}
				json.Unmarshal(join.Description, desc)
				if err = j.pc.SetRemoteDescription(*desc); err != nil {
					j.KillWithError(err)
					return
				}
			}

			if trickle := signal.Signal.GetTrickle(); trickle != nil {
				//log.Debugf("job trickle %s", trickle)
				var candidate webrtc.ICECandidateInit
				_ = json.Unmarshal([]byte(trickle.Init), &candidate)
				err := j.pc.AddICECandidate(candidate)
				if err != nil {
					log.Errorf("error adding ice candidate: %e", err)
				}
			}

			if data := signal.Signal.GetDescription(); data != nil {
				log.Infof("job negotiate %s", data)
				var desc webrtc.SessionDescription
				err := json.Unmarshal(data, &desc)
				if err != nil {
					log.Errorf("Unmarshal negotiate error %s", err)
					continue
				}
				if desc.Type == webrtc.SDPTypeOffer {
					err = j.pc.SetRemoteDescription(desc)
					if err != nil {
						log.Errorf("set offer error %s", err)
						continue
					}
					answer, err := j.pc.CreateAnswer(nil)
					if err != nil {
						log.Errorf("negotiate error %s", err)
						continue
					}

					err = j.pc.SetLocalDescription(answer)
					if err != nil {
						log.Errorf("negotiate error %s", err)
						continue
					}
					answer.Type = webrtc.SDPTypeAnswer
					send, _ := json.Marshal(Negotiation{Desc: answer})
					j.SendSignalRequest(&pb.SignalRequest{
						Payload: &pb.SignalRequest_Description{
							Description: send,
						},
					})
				} else {
					err = j.pc.SetRemoteDescription(desc)
					if err != nil {
						log.Errorf("set answer error %s", err)
						continue
					}
				}
			}
			if signal.Signal.GetKill() {
				log.Debugf("signal killed job=%s", signal.Signal.Id)
				j.Kill(0)
				return
			}
		}
	}

}
