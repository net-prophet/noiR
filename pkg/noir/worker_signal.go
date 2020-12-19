package noir

import (
	"encoding/json"
	"errors"
	pb "github.com/net-prophet/noir/pkg/proto"
	log "github.com/pion/ion-log"
	"github.com/pion/ion-sfu/pkg/sfu"
	"github.com/pion/webrtc/v3"
)

func (w *worker) HandleSignal(request *pb.NoirRequest) error {
	signal := request.GetSignal()
	if request.Action == "request.signal.join" {
		return w.HandleJoin(signal)
	}
	return nil
}

func (w *worker) HandleJoin(signal *pb.SignalRequest) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	mgr := *w.manager

	join := signal.GetJoin()
	pid := signal.Id

	roomData, err := mgr.GetRemoteRoomData(join.Sid)
	options := roomData.GetOptions()

	if err == nil && options.GetMaxPeers() > 0 {
		room := mgr.rooms[join.Sid]
		session := room.Session()
		if session != nil && len(session.Peers()) >= int(options.GetMaxPeers()) {
			return errors.New("room full")
		}
	}

	peer, userData, err := mgr.ConnectUser(signal)

	if err != nil {
		return err
	}

	recv := w.manager.GetQueue(pb.KeyTopicToPeer(pid))
	send := w.manager.GetQueue(pb.KeyTopicFromPeer(pid))

	log.Infof("listening on %s", recv.Topic())

	peer.OnIceCandidate = func(candidate *webrtc.ICECandidateInit, target int) {
		bytes, err := json.Marshal(candidate)
		if err != nil {
			log.Errorf("OnIceCandidate error %s", err)
		}
		err = EnqueueReply(send, &pb.NoirReply{
			Command: &pb.NoirReply_Signal{
				Signal: &pb.SignalReply{
					Id: pid,
					Payload: &pb.SignalReply_Trickle{
						Trickle: &pb.Trickle{
							Init:   string(bytes),
							Target: pb.Trickle_Target(target),
						},
					},
				},
			},
		})
		if err != nil {
			log.Errorf("OnIceCandidate send error %v ", err)
		}

	}

	peer.OnICEConnectionStateChange = func(state webrtc.ICEConnectionState) {

	}

	peer.OnOffer = func(description *webrtc.SessionDescription) {
		bytes, err := json.Marshal(description)
		if err != nil {
			log.Errorf("OnIceCandidate error %s", err)
		}
		err = EnqueueReply(send, &pb.NoirReply{
			Command: &pb.NoirReply_Signal{
				Signal: &pb.SignalReply{
					Id:      pid,
					Payload: &pb.SignalReply_Description{Description: bytes},
				},
			},
		})
		if err != nil {
			log.Errorf("OnIceCandidate send error %v ", err)
		}

	}

	var offer webrtc.SessionDescription
	offer = webrtc.SessionDescription{
		Type: webrtc.SDPTypeOffer,
		SDP:  string(join.Description),
	}

	answer, _ := peer.Join(join.Sid, offer)

	packed, _ := json.Marshal(answer)

	EnqueueReply(send, &pb.NoirReply{
		Command: &pb.NoirReply_Signal{
			Signal: &pb.SignalReply{
				Id:        pid,
				RequestId: signal.RequestId,
				Payload: &pb.SignalReply_Join{
					Join: &pb.JoinReply{
						Description: packed,
					},
				},
			},
		},
	})

	go w.PeerChannel(userData, peer)

	return nil
}

func (w *worker) PeerChannel(userData *pb.UserData, peer *sfu.Peer) {
	recv := w.manager.GetQueue(pb.KeyTopicToPeer(userData.Id))
	send := w.manager.GetQueue(pb.KeyTopicFromPeer(userData.Id))
	for {
		request := pb.NoirRequest{}
		message, err := recv.BlockUntilNext(0)
		if err != nil {
			log.Errorf("getting message to peer %s", err)
		}
		err = UnmarshalRequest(message, &request)
		if err != nil {
			log.Errorf("unmarshal message to peer %s", err)
		}
		switch request.Command.(type) {
		case *pb.NoirRequest_Signal:
			signal := request.GetSignal()
			switch signal.Payload.(type) {
			case *pb.SignalRequest_Kill:
				log.Debugf("got KillRequest for user %s", userData.Id)
				w.manager.DisconnectUser(userData.Id)
				return
			case *pb.SignalRequest_Description:
				var desc Negotiation
				err := json.Unmarshal(signal.GetDescription(), &desc)
				if err != nil {
					log.Errorf("unmarshal err: %s", err)
					continue
				}
				if desc.Desc.Type == webrtc.SDPTypeAnswer {
					log.Debugf("got answer, setting description")
					peer.SetRemoteDescription(desc.Desc)
				} else if desc.Desc.Type == webrtc.SDPTypeOffer {
					roomData, err := w.manager.GetRemoteRoomData(userData.GetRoomID())
					if err != nil {
						log.Errorf("err getting room to validate offer: %s", err)
						continue
					}

					validated, err := w.manager.ValidateOffer(roomData, userData.Id, desc.Desc)

					numTracks := len(validated.MediaDescriptions)
					if numTracks == 1 && validated.MediaDescriptions[0].MediaName.Media == "application" {
						userData.Publishing = false
					} else if numTracks >= 1 {
						// Publishing
						options := roomData.GetOptions()
						if options.GetIsChannel() == true {
							if roomData.GetPublisher() != "" {
								log.Infof("channel already has a publisher, denying: %s", roomData.Id)
								continue
							} else {
								log.Infof("publishing into channel %s", userData.RoomID)
								roomData.Publisher = userData.Id
								SaveRoomData(userData.RoomID, roomData, w.manager)
							}
						}
						userData.Publishing = true
					}

					if err != nil {
						log.Infof("rejected offer: %s", err)
						continue
					}

					answer, _ := peer.Answer(desc.Desc)
					bytes, err := json.Marshal(answer)
					log.Debugf("got offer, sending reply %s", string(bytes))
					err = EnqueueReply(send, &pb.NoirReply{
						Command: &pb.NoirReply_Signal{
							Signal: &pb.SignalReply{
								Id:        userData.Id,
								RequestId: signal.RequestId,
								Payload:   &pb.SignalReply_Description{Description: bytes},
							},
						},
					})
					if err != nil {
						log.Errorf("offer answer send error %v ", err)
					}

					w.manager.SaveData(pb.KeyUserData(userData.Id), &pb.NoirObject{
						Data: &pb.NoirObject_User{User: userData},
					}, 0)
				}
			case *pb.SignalRequest_Trickle:
				trickle := signal.GetTrickle()
				var candidate webrtc.ICECandidateInit
				err := json.Unmarshal([]byte(trickle.GetInit()), &candidate)
				if err != nil {
					log.Errorf("unmarshal err: %s %s", err, trickle.GetInit())
					continue
				}
				peer.Trickle(candidate, int(trickle.Target.Number()))
			default:
				log.Errorf("unknown signal for peer %s", signal.Payload)
			}
		default:
			log.Errorf("unknown command for peer %s", request.Command)
		}
	}
}
