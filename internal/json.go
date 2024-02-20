package internal

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

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

type SkData struct {
	Type       string `json:"type,omitempty"`
	Address    string `json:"address,omitempty"`
	State      string `json:"state,omitempty"`
	Source     string `json:"src,omitempty"`
	SourcePort uint32 `json:"src_port,omitempty"`
	Dest       string `json:"dst,omitempty"`
	DestPort   uint32 `json:"dst_port,omitempty"`
	SendBuf    string `json:"send_buf,omitempty"`
	RecvBuf    string `json:"recv_buf,omitempty"`
}

type SocketNode struct {
	Protocol string `json:"protocol,omitempty"`
	Data     SkData `json:"data,omitempty"`
}

var socketDataFuncs = map[string]func(*crit.Socket) SkData{
	"UNIXSK":    getUnixSkData,
	"INETSK":    getInetSkData,
	"PACKETSK":  getPacketSkData,
	"NETLINKSK": getNetlinkSkData,
}

type DisplayNode struct {
	ContainerName      string         `json:"container_name"`
	Image              string         `json:"image"`
	ID                 string         `json:"id"`
	Runtime            string         `json:"runtime"`
	Created            string         `json:"created"`
	Checkpointed       string         `json:"checkpointed,omitempty"`
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

func RenderJSONView(tasks []Task) error {
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

		if !info.configDump.CheckpointedAt.IsZero() {
			node.Checkpointed = info.configDump.CheckpointedAt.Format(time.RFC3339)
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

		if Stats {
			dumpStats, err := crit.GetDumpStats(task.OutputDir)
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

		if PsTree {
			psTree, err := crit.New(nil, nil, filepath.Join(task.OutputDir, "checkpoint"), false, false).ExplorePs()
			if err != nil {
				return fmt.Errorf("failed to get process tree: %w", err)
			}

			psTreeNode, err := buildJSONPsTree(psTree, task.OutputDir)
			if err != nil {
				return fmt.Errorf("failed to get process tree: %w", err)
			}

			node.ProcessTree = &psTreeNode
		}

		if Files {
			fds, err := crit.New(nil, nil, filepath.Join(task.OutputDir, "checkpoint"), false, false).ExploreFds()
			if err != nil {
				return fmt.Errorf("failed to get file descriptors: %w", err)
			}

			node.FileDescriptors = buildJSONFds(fds)
		}

		if Sockets {
			fds, err := crit.New(nil, nil, filepath.Join(task.OutputDir, "checkpoint"), false, false).ExploreSk()
			if err != nil {
				return fmt.Errorf("failed to get sockets: %w", err)
			}

			node.Sockets, err = buildJSONSks(fds)
			if err != nil {
				return fmt.Errorf("failed to build sockets: %w", err)
			}
		}

		if Mounts {
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

	if PID != 0 {
		ps := psTree.FindPs(PID)
		if ps == nil {
			return PsNode{}, fmt.Errorf("no process with PID %d (use `inspect --ps-tree` to view all PIDs)", PID)
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

	if PsTreeCmd {
		cmdline, err := getCmdline(checkpointOutputDir, psTree.PID)
		if err != nil {
			return PsNode{}, err
		}
		node.Cmdline = cmdline
	}

	if PsTreeEnv {
		envVars, err := getPsEnvVars(checkpointOutputDir, psTree.PID)
		if err != nil {
			return PsNode{}, err
		}
		envVarMap := make(map[string]string)
		for _, envVar := range envVars {
			i := strings.IndexByte(envVar, '=')
			if i == -1 || i == 0 {
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

func buildJSONSks(sks []*crit.Sk) ([]SkNode, error) {
	var result []SkNode

	for _, sk := range sks {
		skNode := SkNode{
			PID: sk.PId,
		}

		var sockets []SocketNode
		for _, socket := range sk.Sockets {
			socketData, err := getDataForSocket(socket)
			if err != nil {
				return nil, fmt.Errorf("error getting data for socket: %w", err)
			}
			sockets = append(sockets, SocketNode{
				Protocol: socket.Protocol,
				Data:     socketData,
			})
		}

		if len(sockets) > 0 {
			skNode.OpenSockets = sockets
		}

		result = append(result, skNode)
	}

	return result, nil
}

func getDataForSocket(socket *crit.Socket) (SkData, error) {
	dataFunc, found := socketDataFuncs[socket.FdType]
	if !found {
		return SkData{}, fmt.Errorf("unsupported socket type: %s", socket.FdType)
	}
	return dataFunc(socket), nil
}

func getUnixSkData(socket *crit.Socket) SkData {
	return SkData{
		Type:    "UNIX",
		Address: socket.SrcAddr,
	}
}

func getInetSkData(socket *crit.Socket) SkData {
	return SkData{
		Type:       socket.Protocol,
		State:      socket.State,
		Source:     socket.SrcAddr,
		SourcePort: socket.SrcPort,
		Dest:       socket.DestAddr,
		DestPort:   socket.DestPort,
		SendBuf:    socket.SendBuf,
		RecvBuf:    socket.RecvBuf,
	}
}

func getPacketSkData(socket *crit.Socket) SkData {
	return SkData{
		SendBuf: socket.SendBuf,
		RecvBuf: socket.RecvBuf,
	}
}

func getNetlinkSkData(socket *crit.Socket) SkData {
	return SkData{
		SendBuf: socket.SendBuf,
		RecvBuf: socket.RecvBuf,
	}
}
