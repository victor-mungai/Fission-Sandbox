package pool

import "fmt"

type VMState string

const (
	VMStateIdle      VMState = "IDLE"
	VMStateBusy      VMState = "BUSY"
	VMStateResetting VMState = "RESETTING"
	VMStateEvicted   VMState = "EVICTED"
)

func canTransitionVMState(from VMState, to VMState) bool {
	switch from {
	case VMStateIdle:
		return to == VMStateBusy || to == VMStateEvicted
	case VMStateBusy:
		return to == VMStateResetting || to == VMStateEvicted
	case VMStateResetting:
		return to == VMStateIdle || to == VMStateEvicted
	case VMStateEvicted:
		return false
	default:
		return false
	}
}

func validateVMStateTransition(from VMState, to VMState) error {
	if canTransitionVMState(from, to) {
		return nil
	}
	return fmt.Errorf("invalid vm state transition: %s -> %s", from, to)
}
