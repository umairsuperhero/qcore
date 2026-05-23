package udm

import "sync"

// uecmStore is an in-memory serving-AMF registry keyed by ueId (SUPI).
type uecmStore struct {
	mu   sync.RWMutex
	regs map[string]AmfRegistration
}

func newUECMStore() *uecmStore {
	return &uecmStore{regs: make(map[string]AmfRegistration)}
}

func (u *uecmStore) put(ueID string, reg AmfRegistration) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.regs[ueID] = reg
}

func (u *uecmStore) get(ueID string) (AmfRegistration, bool) {
	u.mu.RLock()
	defer u.mu.RUnlock()
	r, ok := u.regs[ueID]
	return r, ok
}

func (u *uecmStore) delete(ueID string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	delete(u.regs, ueID)
}
