[![main](https://github.com/checkpoint-restore/checkpointctl/workflows/Run%20Tests/badge.svg?branch=main)](https://github.com/checkpoint-restore/checkpointctl/actions)

## checkpointctl -- Work with Kubernetes checkpoints

The Kubernetes kubelet creates checkpoints which consists of metadata (`checkpointed.pods`)
and tar archives containing the actual pod checkpoints.

With the help of this tool (`checkpointctl`) it is possible to display, extract or insert
checkpoints.

To display the checkpoints which are currently in the kubelet's default checkpoint directory
just use `checkpointctl show`:

```shell
$ checkpointctl show

Displaying data from /var/lib/kubelet/checkpoints/checkpointed.pods

+-----------+-----------+-----------+-----------------------------------+---------------+
|    POD    | NAMESPACE | CONTAINER |               IMAGE               | ARCHIVE FOUND |
+-----------+-----------+-----------+-----------------------------------+---------------+
| my-redis  | default   | redis     | redis                             | true          |
+-----------+           +-----------+-----------------------------------+               +
| counters  |           | counter   | quay.io/adrianreber/counter       |               |
+           +           +-----------+-----------------------------------+               +
|           |           | wildfly   | quay.io/adrianreber/wildfly-hello |               |
+-----------+-----------+-----------+-----------------------------------+---------------+
```

To extract all checkpoints from the kubelet use:

```shell
$ checkpointctl extract -o /tmp/checkpoints.tar

Extracting checkpoint data from /var/lib/kubelet/checkpoints/checkpointed.pods

```

Resulting in a tar archive at `/tmp/checkpoints.tar` which can then be used to insert
this checkpoint archive into another kubelet:

```shell
$ checkpointctl insert -i /tmp/checkpoints.tar
```

Inserting a checkpoint archive will add the new checkpoints to existing checkpoints.

All operations default to `/var/lib/kubelet/checkpoints` which can be changed using
the `--target` parameter.

The command `checkpointctl show` can also be used on an exported tar archive to see
which checkpoints are part of an exported tar archive:

```
$ checkpointctl show --target /tmp/checkpoints.tar
```

### How to contribute

While bug fixes can first be identified via an "issue", that is not required.
It's ok to just open up a PR with the fix, but make sure you include the same
information you would have included in an issue - like how to reproduce it.

PRs for new features should include some background on what use cases the
new code is trying to address. When possible and when it makes sense, try to
break-up larger PRs into smaller ones - it's easier to review smaller
code changes. But only if those smaller ones make sense as stand-alone PRs.

Regardless of the type of PR, all PRs should include:
* well documented code changes
* additional testcases. Ideally, they should fail w/o your code change applied
* documentation changes

Squash your commits into logical pieces of work that might want to be reviewed
separate from the rest of the PRs. Ideally, each commit should implement a
single idea, and the PR branch should pass the tests at every commit. GitHub
makes it easy to review the cumulative effect of many commits; so, when in
doubt, use smaller commits.

PRs that fix issues should include a reference like `Closes #XXXX` in the
commit message so that github will automatically close the referenced issue
when the PR is merged.

Contributors must assert that they are in compliance with the [Developer
Certificate of Origin 1.1](http://developercertificate.org/). This is achieved
by adding a "Signed-off-by" line containing the contributor's name and e-mail
to every commit message. Your signature certifies that you wrote the patch or
otherwise have the right to pass it on as an open-source patch.

### License and copyright

Unless mentioned otherwise in a specific file's header, all code in
this project is released under the Apache 2.0 license.

The author of a change remains the copyright holder of their code
(no copyright assignment). The list of authors and contributors can be
retrieved from the git commit history and in some cases, the file headers.
