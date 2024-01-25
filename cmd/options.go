package cmd

import "github.com/checkpoint-restore/checkpointctl/internal"

var (
	format         *string = &internal.Format
	stats          *bool   = &internal.Stats
	mounts         *bool   = &internal.Mounts
	outputFilePath *string = &internal.OutputFilePath
	pID            *uint32 = &internal.PID
	psTree         *bool   = &internal.PsTree
	psTreeCmd      *bool   = &internal.PsTreeCmd
	psTreeEnv      *bool   = &internal.PsTreeEnv
	files          *bool   = &internal.Files
	sockets        *bool   = &internal.Sockets
	showAll        *bool   = &internal.ShowAll
)
