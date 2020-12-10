package proto

func KeyRoomToWorker(roomID string) string {
	return "noir/room2worker/" + roomID
}

func KeyPeerToRoom(peerID string) string {
	return "noir/peer2room/" + peerID
}

func KeyRoomStatus(roomID string) string {
	return "noir/room.status/" + roomID
}

func KeyWorkerStatus(workerID string) string {
	return "noir/worker.status/" + workerID
}

func KeyWorkerTopic(workerID string) string {
	return "noir/worker.topic/" + workerID
}

func KeyPeerStatus(peerID string) string {
	return "noir/peer.status/" + peerID
}