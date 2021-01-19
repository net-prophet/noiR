package noir

import (
	"encoding/json"
	"errors"
	pb "github.com/net-prophet/noir/pkg/proto"
	log "github.com/pion/ion-log"
	"github.com/pion/ion-sfu/pkg/sfu"
	"github.com/pion/sdp/v3"
	"github.com/pion/webrtc/v3"
)

func (w *worker) HandleSignal(request *pb.NoirRequest) error {
	signal := request.GetSignal()
	if signal.GetJoin() != nil {
		return w.HandleJoin(request)
	}
	return nil
}

func (w *worker) HandleJoin(request *pb.NoirRequest) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	mgr := *w.manager

	signal := request.GetSignal()
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

	log.Infof("listening on %s", recv.Topic())

	peer.OnIceCandidate = func(candidate *webrtc.ICECandidateInit, target int) {
		bytes, err := json.Marshal(candidate)
		if err != nil {
			log.Errorf("OnIceCandidate error %s", err)
		}
		w.SignalReply(pid, &pb.NoirReply{
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
		w.SignalReply(pid, &pb.NoirReply{
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

	w.manager.UpdateRoomScore(join.Sid)

	packed, _ := json.Marshal(answer)

	w.SignalReply(pid, &pb.NoirReply{
		Id: request.Id,
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

func (w *worker) SignalReply(pid string, reply *pb.NoirReply) error {
	send := w.manager.GetQueue(pb.KeyTopicFromPeer(pid))
	defer w.manager.redis.Publish(pb.KeyPeerNewsChannel(pid), pid)
	return EnqueueReply(send, reply)
}

func (w *worker) PeerChannel(userData *pb.UserData, peer *sfu.Peer) {
	recv := w.manager.GetQueue(pb.KeyTopicToPeer(userData.Id))
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

					A, V, D, summary := TrackSummary(validated)

					roomType := "room"

					// Just one data track
					if D == 1 && A == 0 && V == 0 {
						userData.Publishing = false
					} else if A > 0 || V > 0 {
						// Publishing
						options := roomData.GetOptions()

						if options.GetIsChannel() == true {
							roomType = "channel"
							if roomData.GetPublisher() != "" {
								log.Infof("channel already has a publisher, denying: %s", roomData.Id)
								continue
							} else if A > 1 || V > 1 {
								log.Infof("cannot publish multiple video or audio tracks into channel %s: %s", roomData.Id, summary)
								continue
							} else {
								roomData.Publisher = userData.Id
								SaveRoomData(userData.RoomID, roomData, w.manager)
							}
						}
						userData.Publishing = true
						log.Infof("publishing [%dA/%dV/%dD] into %s %s: %s", A, V, D, roomType, userData.RoomID, summary)
					}

					if err != nil {
						log.Infof("rejected offer: %s", err)
						continue
					}

					answer, _ := peer.Answer(desc.Desc)
					bytes, err := json.Marshal(answer)
					log.Debugf("got offer %s, sending reply %s", request.Id, summary)
					w.SignalReply(userData.Id, &pb.NoirReply{
						Id: request.Id,
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
				log.Errorf("unknown servers for peer %s", signal.Payload)
			}
		default:
			log.Errorf("unknown command for peer %s", request.Command)
		}
	}
}


func TrackSummary(desc *sdp.SessionDescription) (int, int, int, string) {
	summary := ""
	audioTracks, videoTracks, dataTracks := 0, 0, 0

	for _, track := range desc.MediaDescriptions {
		media := track.MediaName
		switch media.Media {
		case "application":
			dataTracks += 1
		case "audio":
			audioTracks += 1
		case "video":
			videoTracks += 1
		}

		if summary != "" {
			summary += ", "
		}
		summary += media.Media + "["
		fmts := ""
		if rtpmap, exists := track.Attribute("rtpmap"); exists {
			summary += rtpmap
		} else {
			for _, fmt := range media.Formats {
				if fmts != "" {
					fmts += ", "
				}
				fmts += fmt
			}
			summary += fmts
		}
		summary += "]"

	}
	return audioTracks, videoTracks, dataTracks, summary
}
