// Copyright 2015 Muir Manders.  All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package goftp

// Taken from https://www.ietf.org/rfc/rfc959.txt

const (
	// positive preliminary replies
	ReplyRestartMarker             = 110 // Restart marker reply
	ReplyReadyInNMinutes           = 120 // Service ready in nnn minutes
	ReplyDataConnectionAlreadyOpen = 125 // (transfer starting)
	ReplyFileStatusOkay            = 150 // (about to open data connection)

	// positive completion replies
	ReplyCommandOkay               = 200
	ReplyCommandOkayNotImplemented = 202
	ReplySystemStatus              = 211 // or system help reply
	ReplyDirectoryStatus           = 212
	ReplyFileStatus                = 213
	ReplyHelpMessage               = 214
	ReplySystemType                = 215
	ReplyServiceReady              = 220
	ReplyClosingControlConnection  = 221
	ReplyDataConnectionOpen        = 225 // (no transfer in progress)
	ReplyClosingDataConnection     = 226 // requested file action successful
	ReplyEnteringPassiveMode       = 227
	ReplyUserLoggedIn              = 230
	ReplyFileActionOkay            = 250 // (completed)
	ReplyDirCreated                = 257

	// positive intermediate replies
	ReplyNeedPassword      = 331
	ReplyNeedAccount       = 332
	ReplyFileActionPending = 350 // pending further information

	// transient negative completion replies
	ReplyServiceNotAvailable    = 421 // (service shutting down)
	ReplyCantOpenDataConnection = 425
	ReplyConnectionClosed       = 426 // (transfer aborted)
	ReplyTransientFileError     = 450 // (file unavailable)
	ReplyLocalError             = 451 // action aborted
	ReplyOutOfSpace             = 452 // action not taken

	// permanenet negative completion replies
	ReplyCommandSyntaxError                = 500
	ReplyParameterSyntaxError              = 501
	ReplyCommandNotImplemented             = 502
	ReplyBadCommandSequence                = 503
	ReplyCommandNotImplementedForParameter = 504
	ReplyNotLoggedIn                       = 530
	ReplyNeedAccountToStore                = 532
	ReplyFileError                         = 550 // file not found, no access
	ReplyPageTypeUnknown                   = 551
	ReplyExceededStorageAllocation         = 552 // for current directory/dataset
	ReplyBadFileName                       = 553
)

func positiveCompletionReply(code int) bool {
	return code/100 == 2
}

func positivePreliminaryReply(code int) bool {
	return code/100 == 1
}
