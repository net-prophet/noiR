package proto

func KeyRoomToWorker(roomID string) string {
	return "noir/room2worker/" + roomID
}

func KeyPeerToRoom(peerID string) string {
	return "noir/peer2room/" + peerID
}

func KeyRoomStatus(roomID string) string {
	return "noir/status.room/" + roomID
}

func KeyWorkerStatus(workerID string) string {
	return "noir/status.worker/" + workerID
}

func KeyWorkerTopic(workerID string) string {
	return "noir/topic.to.worker/" + workerID
}

func KeyPeerStatus(peerID string) string {
	return "noir/status.peer/" + peerID
}

func KeyTopicToPeer(peerID string) string {
	return "noir/topic.to.pc/" + peerID
}

func KeyTopicFromPeer(peerID string) string {
	return "noir/topic.from.pc/" + peerID
}
