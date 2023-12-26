package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/checkpoint-restore/go-criu/v7/crit"
	spec "github.com/opencontainers/runtime-spec/specs-go"
)

type CheckpointSize struct {
	TotalSize             int64 `json:"total_size,omitempty"`
	MemoryPagesSize       int64 `json:"memory_pages_size,omitempty"`
	AmdGpuMemoryPagesSize int64 `json:"amd_gpu_memory_pages_size,omitempty"`
	RootFsDiffSize        int64 `json:"root_fs_diff_size,omitempty"`
}

type StatsNode struct {
	FreezingTime uint32 `json:"freezing_time,omitempty"`
	FrozenTime   uint32 `json:"frozen_time,omitempty"`
	MemdumpTime  uint32 `json:"memdump_time,omitempty"`
	MemwriteTime uint32 `json:"memwrite_time,omitempty"`
	PagesScanned uint64 `json:"pages_scanned,omitempty"`
	PagesWritten uint64 `json:"pages_written,omitempty"`
}

type PsNode struct {
	PID      uint32            `json:"pid"`
	Comm     string            `json:"command"`
	Cmdline  string            `json:"cmdline,omitempty"`
	EnvVars  map[string]string `json:"environment_variables,omitempty"`
	Children []PsNode          `json:"children,omitempty"`
}

type FdNode struct {
	PID       uint32         `json:"pid"`
	OpenFiles []OpenFileNode `json:"open_files,omitempty"`
}

type OpenFileNode struct {
	Type string `json:"type"`
	FD   string `json:"fd"`
	Path string `json:"path"`
}

type SkNode struct {
	PID         uint32       `json:"pid"`
	OpenSockets []SocketNode `json:"open_sockets,omitempty"`
}

type SocketNode struct {
	Protocol string `json:"protocol"`
	Data     string `json:"data"`
}

type DisplayNode struct {
	ContainerName      string         `json:"container_name"`
	Image              string         `json:"image"`
	ID                 string         `json:"id"`
	Runtime            string         `json:"runtime"`
	Created            string         `json:"created"`
	Engine             string         `json:"engine"`
	IP                 string         `json:"ip,omitempty"`
	MAC                string         `json:"mac,omitempty"`
	CheckpointSize     CheckpointSize `json:"checkpoint_size"`
	CriuDumpStatistics *StatsNode     `json:"statistics,omitempty"`
	ProcessTree        *PsNode        `json:"process_tree,omitempty"`
	FileDescriptors    []FdNode       `json:"file_descriptors,omitempty"`
	Sockets            []SkNode       `json:"sockets,omitempty"`
	Mounts             []MountNode    `json:"mounts,omitempty"`
}

type MountNode struct {
	Destination string `json:"destination"`
	Type        string `json:"type"`
	Source      string `json:"source"`
}

func renderJSONView(tasks []task) error {
	var result []DisplayNode

	for _, task := range tasks {
		info, err := getCheckpointInfo(task)
		if err != nil {
			return err
		}

		node := DisplayNode{
			ContainerName: info.containerInfo.Name,
			Image:         info.configDump.RootfsImageName,
			ID:            info.configDump.ID,
			Runtime:       info.configDump.OCIRuntime,
			Created:       info.containerInfo.Created,
			Engine:        info.containerInfo.Engine,
		}

		if info.containerInfo.IP != "" {
			node.IP = info.containerInfo.IP
		}
		if info.containerInfo.MAC != "" {
			node.MAC = info.containerInfo.MAC
		}

		checkpointSizeNode := CheckpointSize{
			TotalSize: info.archiveSizes.checkpointSize,
		}

		if info.archiveSizes.pagesSize != 0 {
			checkpointSizeNode.MemoryPagesSize = info.archiveSizes.pagesSize
		}

		if info.archiveSizes.amdgpuPagesSize != 0 {
			checkpointSizeNode.AmdGpuMemoryPagesSize = info.archiveSizes.amdgpuPagesSize
		}

		if info.archiveSizes.rootFsDiffTarSize != 0 {
			checkpointSizeNode.RootFsDiffSize = info.archiveSizes.rootFsDiffTarSize
		}

		node.CheckpointSize = checkpointSizeNode

		if stats {
			dumpStats, err := crit.GetDumpStats(task.outputDir)
			if err != nil {
				return fmt.Errorf("failed to get dump statistics: %w", err)
			}

			statsNode := StatsNode{
				FreezingTime: dumpStats.GetFreezingTime(),
				FrozenTime:   dumpStats.GetFrozenTime(),
				MemdumpTime:  dumpStats.GetMemdumpTime(),
				MemwriteTime: dumpStats.GetMemwriteTime(),
				PagesScanned: dumpStats.GetPagesScanned(),
				PagesWritten: dumpStats.GetPagesWritten(),
			}

			node.CriuDumpStatistics = &statsNode
		}

		if psTree {
			psTree, err := crit.New(nil, nil, filepath.Join(task.outputDir, "checkpoint"), false, false).ExplorePs()
			if err != nil {
				return fmt.Errorf("failed to get process tree: %w", err)
			}

			psTreeNode, err := buildJSONPsTree(psTree, task.outputDir)
			if err != nil {
				return fmt.Errorf("failed to get process tree: %w", err)
			}

			node.ProcessTree = &psTreeNode
		}

		if files {
			fds, err := crit.New(nil, nil, filepath.Join(task.outputDir, "checkpoint"), false, false).ExploreFds()
			if err != nil {
				return fmt.Errorf("failed to get file descriptors: %w", err)
			}

			node.FileDescriptors = buildJSONFds(fds)
		}

		if sockets {
			fds, err := crit.New(nil, nil, filepath.Join(task.outputDir, "checkpoint"), false, false).ExploreSk()
			if err != nil {
				return fmt.Errorf("failed to get sockets: %w", err)
			}

			node.Sockets = buildJSONSks(fds)
		}

		if mounts {
			specDump := info.specDump
			node.Mounts = buildJSONMounts(specDump)
		}

		result = append(result, node)
	}

	jsonData, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}

	fmt.Printf("%s\n", jsonData)

	return nil
}

func buildJSONMounts(specDump *spec.Spec) []MountNode {
	var result []MountNode

	for _, data := range specDump.Mounts {
		mountNode := MountNode{
			Destination: data.Destination,
			Type:        data.Type,
			Source:      data.Source,
		}
		result = append(result, mountNode)
	}

	return result
}

func buildJSONPsTree(psTree *crit.PsTree, checkpointOutputDir string) (PsNode, error) {
	var rootNode PsNode
	var err error

	if pID != 0 {
		ps := psTree.FindPs(pID)
		if ps == nil {
			return PsNode{}, fmt.Errorf("no process with PID %d (use `inspect --ps-tree` to view all PIDs)", pID)
		}
		rootNode, err = buildJSONPsNode(ps, checkpointOutputDir)
	} else {
		rootNode, err = buildJSONPsNode(psTree, checkpointOutputDir)
	}

	if err != nil {
		return PsNode{}, fmt.Errorf("failed to build JSON process tree node: %w", err)
	}

	return rootNode, nil
}

func buildJSONPsNode(psTree *crit.PsTree, checkpointOutputDir string) (PsNode, error) {
	node := PsNode{
		PID:  psTree.PID,
		Comm: psTree.Comm,
	}

	if psTreeCmd {
		cmdline, err := getCmdline(checkpointOutputDir, psTree.PID)
		if err != nil {
			return PsNode{}, err
		}
		node.Cmdline = cmdline
	}

	if psTreeEnv {
		envVars, err := getPsEnvVars(checkpointOutputDir, psTree.PID)
		if err != nil {
			return PsNode{}, err
		}
		envVarMap := make(map[string]string)
		for _, envVar := range envVars {
			i := strings.IndexByte(envVar, '=')
			if i == -1 || i == 0 || i == len(envVar)-1 {
				return PsNode{}, fmt.Errorf("invalid environment variable %s", envVar)
			}
			key := envVar[:i]
			value := envVar[i+1:]
			envVarMap[key] = value
		}

		node.EnvVars = envVarMap
	}

	var children []PsNode
	for _, child := range psTree.Children {
		childNode, err := buildJSONPsNode(child, checkpointOutputDir)
		if err != nil {
			return PsNode{}, err
		}
		children = append(children, childNode)
	}

	if len(children) > 0 {
		node.Children = children
	}

	return node, nil
}

func buildJSONFds(fds []*crit.Fd) []FdNode {
	var result []FdNode

	for _, fd := range fds {
		fdNode := FdNode{
			PID: fd.PId,
		}

		var files []OpenFileNode
		for _, file := range fd.Files {
			fileNode := OpenFileNode{
				Type: file.Type,
				FD:   strings.TrimSpace(file.Type + " " + file.Fd),
				Path: file.Path,
			}
			files = append(files, fileNode)
		}

		if len(files) > 0 {
			fdNode.OpenFiles = files
		}

		result = append(result, fdNode)
	}

	return result
}

func buildJSONSks(sks []*crit.Sk) []SkNode {
	var result []SkNode

	for _, sk := range sks {
		skNode := SkNode{
			PID: sk.PId,
		}

		var sockets []SocketNode
		for _, socket := range sk.Sockets {
			socketNode := SocketNode{
				Protocol: socket.Protocol,
				Data:     getDataForSocket(socket),
			}
			sockets = append(sockets, socketNode)
		}

		if len(sockets) > 0 {
			skNode.OpenSockets = sockets
		}

		result = append(result, skNode)
	}

	return result
}

func getDataForSocket(socket *crit.Socket) string {
	switch socket.FdType {
	case "UNIXSK":
		return getUnixSkData(socket)
	case "INETSK":
		return getInetSkData(socket)
	case "PACKETSK":
		return getPacketSkData(socket)
	case "NETLINKSK":
		return getNetlinkSkData(socket)
	default:
		return ""
	}
}

func getUnixSkData(socket *crit.Socket) string {
	return fmt.Sprintf("Type: UNIX (%s), Address: %s", socket.Type, socket.SrcAddr)
}

func getInetSkData(socket *crit.Socket) string {
	return fmt.Sprintf(
		"Type: %s, State: %s, Source: %s:%d, Destination: %s:%d, SendBuf: %s, RecvBuf: %s",
		socket.Protocol, socket.State,
		socket.SrcAddr, socket.SrcPort,
		socket.DestAddr, socket.DestPort,
		socket.SendBuf, socket.RecvBuf,
	)
}

func getPacketSkData(socket *crit.Socket) string {
	return fmt.Sprintf("Type: PACKET, SendBuf: %s, RecvBuf: %s", socket.SendBuf, socket.RecvBuf)
}

func getNetlinkSkData(socket *crit.Socket) string {
	return fmt.Sprintf("Type: NETLINK, SendBuf: %s, RecvBuf: %s", socket.SendBuf, socket.RecvBuf)
}
