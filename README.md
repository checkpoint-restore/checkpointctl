<!-- markdownlint-disable MD013 -->
# checkpointctl -- Show information about checkpoint archives

[![Run Tests](https://github.com/checkpoint-restore/checkpointctl/actions/workflows/tests.yml/badge.svg)](https://github.com/checkpoint-restore/checkpointctl/actions/workflows/tests.yml)

Container engines like *Podman* and *CRI-O* have the ability to checkpoint a
container.  All data related to a checkpoint is collected in a checkpoint
archive. With the help of this tool, `checkpointctl`, it is possible to display
information about these checkpoint archives.

Details on how to create checkpoints with the help of [CRIU][criu] can be found at:

* [Forensic container checkpointing in Kubernetes][forensic]
* [Podman checkpoint][podman]

To display information about a checkpoint archive you can just use
`checkpointctl show`:

```console
$ checkpointctl show /tmp/dump.tar

+-----------------+------------------------------------------+--------------+---------+----------------------+--------+------------+-------------------+
|    CONTAINER    |                  IMAGE                   |      ID      | RUNTIME |       CREATED        | ENGINE | CHKPT SIZE | ROOT FS DIFF SIZE |
+-----------------+------------------------------------------+--------------+---------+----------------------+--------+------------+-------------------+
| magical_murdock | quay.io/adrianreber/wildfly-hello:latest | f11d11844af0 | crun    | 2023-02-28T09:43:52Z | Podman | 338.2 MiB  | 177.0 KiB         |
+-----------------+------------------------------------------+--------------+---------+----------------------+--------+------------+-------------------+
```

For a checkpoint archive created by Kubernetes with *CRI-O* the output would
look like this:

```console
$ checkpointctl show /var/lib/kubelet/checkpoints/checkpoint-counters_default-counter-2023-02-13T16\:20\:09Z.tar

+-----------+------------------------------------+--------------+---------+--------------------------------+--------+------------+------------+
| CONTAINER |               IMAGE                |      ID      | RUNTIME |            CREATED             | ENGINE |     IP     | CHKPT SIZE |
+-----------+------------------------------------+--------------+---------+--------------------------------+--------+------------+------------+
| counter   | quay.io/adrianreber/counter:latest | 7eb9680287f1 | runc    | 2023-02-13T16:12:25.843774934Z | CRI-O  | 10.88.0.24 | 8.5 MiB    |
+-----------+------------------------------------+--------------+---------+--------------------------------+--------+------------+------------+
```

It is also possible to display additional checkpoint related information
with the parameter `--print-stats`:

```console
$ checkpointctl show /tmp/dump.tar --print-stats

+-----------------+------------------------------------------+--------------+---------+----------------------+--------+------------+-------------------+
|    CONTAINER    |                  IMAGE                   |      ID      | RUNTIME |       CREATED        | ENGINE | CHKPT SIZE | ROOT FS DIFF SIZE |
+-----------------+------------------------------------------+--------------+---------+----------------------+--------+------------+-------------------+
| magical_murdock | quay.io/adrianreber/wildfly-hello:latest | f11d11844af0 | crun    | 2023-02-28T09:43:52Z | Podman | 338.2 MiB  | 177.0 KiB         |
+-----------------+------------------------------------------+--------------+---------+----------------------+--------+------------+-------------------+
CRIU dump statistics
+---------------+-------------+--------------+---------------+---------------+---------------+
| FREEZING TIME | FROZEN TIME | MEMDUMP TIME | MEMWRITE TIME | PAGES SCANNED | PAGES WRITTEN |
+---------------+-------------+--------------+---------------+---------------+---------------+
| 104450 us     | 442148 us   | 212281 us    | 148292 us     |        495649 |         86510 |
+---------------+-------------+--------------+---------------+---------------+---------------+
```

## How to contribute

While bug fixes can first be identified via an "issue", that is not required.
It's ok to just open up a PR with the fix, but make sure you include the same
information you would have included in an issue - like how to reproduce it.

PRs for new features should include some background on what use cases the
new code is trying to address. When possible and when it makes sense, try to
break-up larger PRs into smaller ones - it's easier to review smaller
code changes. But only if those smaller ones make sense as stand-alone PRs.

Regardless of the type of PR, all PRs should include:

* well documented code changes;
* additional testcases: ideally, they should fail w/o your code change applied;
* documentation changes.

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

## License and copyright

Unless mentioned otherwise in a specific file's header, all code in
this project is released under the Apache 2.0 license.

The author of a change remains the copyright holder of their code
(no copyright assignment). The list of authors and contributors can be
retrieved from the git commit history and in some cases, the file headers.

[forensic]: https://kubernetes.io/blog/2022/12/05/forensic-container-checkpointing-alpha/
[podman]: https://podman.io/getting-started/checkpoint
[criu]: https://criu.org/
