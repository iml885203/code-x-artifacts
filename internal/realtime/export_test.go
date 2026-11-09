package realtime

import "weak"

// LoadedRoomForTest returns a weak reference to a loaded room, so a test can
// ask the garbage collector whether anything still holds it after an unload.
func LoadedRoomForTest(h *Hub, id string) weak.Pointer[room] {
	return weak.Make(h.loadedRoom(id))
}
