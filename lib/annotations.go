package metadata

const (
	// CheckpointAnnotationContainerManager is used when creating an OCI image
	// from a checkpoint archive to specify the name of container manager.
	CheckpointAnnotationContainerManager = "checkpointctl.annotation.container.manager"

	// CheckpointAnnotationName is used when creating an OCI image
	// from a checkpoint archive to specify the name of the checkpoint.
	CheckpointAnnotationName = "checkpointctl.annotation.checkpoint.name"

	// CheckpointAnnotationPod is used when creating an OCI image
	// from a checkpoint archive to specify the name of the pod associated with the checkpoint.
	CheckpointAnnotationPod = "checkpointctl.annotation.checkpoint.pod"

	// CheckpointAnnotationNamespace is used when creating an OCI image
	// from a checkpoint archive to specify the namespace of the pod associated with the checkpoint.
	CheckpointAnnotationNamespace = "checkpointctl.annotation.checkpoint.namespace"

	// CheckpointAnnotationRootfsImage is used when creating an OCI image
	// from a checkpoint archive to specify the root filesystem image associated with the checkpoint.
	CheckpointAnnotationRootfsImage = "checkpointctl.annotation.checkpoint.rootfsImage"

	// CheckpointAnnotationRootfsImageID is used when creating an OCI image
	// from a checkpoint archive to specify the ID of the root filesystem image associated with the checkpoint.
	CheckpointAnnotationRootfsImageID = "checkpointctl.annotation.checkpoint.rootfsImageID"

	// CheckpointAnnotationRootfsImageName is used when creating an OCI image
	// from a checkpoint archive to specify the name of the root filesystem image associated with the checkpoint.
	CheckpointAnnotationRootfsImageName = "checkpointctl.annotation.checkpoint.rootfsImageName"

	// CheckpointAnnotationRuntimeName is used when creating an OCI image
	// from a checkpoint archive to specify the runtime used on the host where the checkpoint was created.
	CheckpointAnnotationRuntimeName = "checkpointctl.annotation.checkpoint.runtime.name"
)
