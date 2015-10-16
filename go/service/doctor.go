package service

import (
	"github.com/keybase/client/go/engine"
	"github.com/keybase/client/go/libkb"
	keybase1 "github.com/keybase/client/go/protocol"
	rpc "github.com/keybase/go-framed-msgpack-rpc"
)

// DoctorHandler implements the keybase_1.Doctor protocol
type DoctorHandler struct {
	*BaseHandler
	libkb.Contextified
}

func NewDoctorHandler(xp rpc.Transporter, g *libkb.GlobalContext) *DoctorHandler {
	return &DoctorHandler{
		BaseHandler:  NewBaseHandler(xp),
		Contextified: libkb.NewContextified(g),
	}
}

func (h *DoctorHandler) Doctor(sessionID int) error {
	ctx := &engine.Context{
		DoctorUI:    h.ui(sessionID),
		LogUI:       h.getLogUI(sessionID),
		SecretUI:    h.getSecretUI(sessionID),
		LoginUI:     h.getLoginUI(sessionID),
		LocksmithUI: h.getLocksmithUI(sessionID),
		GPGUI:       h.getGPGUI(sessionID),
	}
	eng := engine.NewDoctor(h.G())
	return engine.RunEngine(eng, ctx)
}

func (h *DoctorHandler) ui(sessionID int) *RemoteDoctorUI {
	c := h.rpcClient()
	return &RemoteDoctorUI{
		sessionID: sessionID,
		uicli:     keybase1.DoctorUiClient{Cli: c},
	}
}

type RemoteDoctorUI struct {
	sessionID int
	uicli     keybase1.DoctorUiClient
}

func (r *RemoteDoctorUI) LoginSelect(currentUser string, otherUsers []string) (string, error) {
	return r.uicli.LoginSelect(keybase1.LoginSelectArg{
		SessionID:   r.sessionID,
		CurrentUser: currentUser,
		OtherUsers:  otherUsers,
	})
}

func (r *RemoteDoctorUI) DisplayStatus(status keybase1.DoctorStatus) (bool, error) {
	return r.uicli.DisplayStatus(keybase1.DisplayStatusArg{
		SessionID: r.sessionID,
		Status:    status,
	})
}

func (r *RemoteDoctorUI) DisplayResult(msg string) error {
	return r.uicli.DisplayResult(keybase1.DisplayResultArg{
		SessionID: r.sessionID,
		Message:   msg,
	})
}
