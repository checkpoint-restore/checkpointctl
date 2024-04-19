<!-- markdownlint-disable MD033 -->
<!-- markdownlint-disable MD041 -->
<p1 align="center"><img src="docs/checkpointctl.svg" height="250"></p>

<!-- markdownlint-disable MD013 -->
# checkpointctl - a tool for in-depth analysis of container checkpoints

[![Run Tests](https://github.com/checkpoint-restore/checkpointctl/actions/workflows/tests.yml/badge.svg)](https://github.com/checkpoint-restore/checkpointctl/actions/workflows/tests.yml)

Container engines like *Podman* and *CRI-O* have the ability to checkpoint a
container.  All data related to a checkpoint is collected in a checkpoint
archive. With the help of this tool, `checkpointctl`, it is possible to display
information about these checkpoint archives.

Details on how to create checkpoints with the help of [CRIU][criu] can be found at:

* [Forensic container checkpointing in Kubernetes][forensic]
* [Podman checkpoint][podman]

## Usage

### `show` sub-command

To display an overview of a checkpoint archive you can just use
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

### `inspect` sub-command

To retrieve low-level information about a container checkpoint, use the `checkpointctl inspect` command:

```console
$ checkpointctl inspect /tmp/ubuntu_looper.tar.gz --ps-tree

awesome_booth
├── Image: docker.io/library/ubuntu:latest
├── ID: 695b77deb38281244a114da111e2ee606ab9464ffa94a98be382d181c2121c9c
├── Runtime: crun
├── Created: 2023-03-08T08:45:33+03:00
├── Engine: Podman
├── Checkpoint size: 2.8 MiB
├── Root FS diff size: 309.0 KiB
└── Process tree
    └── [1]  bash
        └── [5]  su
            └── [6]  bash
                └── [47]  loop.sh
                    └── [74]  sleep
```

For a complete list of flags supported, use `checkpointctl inspect --help`.

### `memparse` sub-command

To perform memory analysis of container checkpoints, you can use the `checkpointctl memparse` command.

```console
$ checkpointctl memparse /tmp/jira.tar.gz  --pid=1 | less

Displaying memory pages content for Process ID 1 from checkpoint: /tmp/jira.tar.gz

Address           Hexadecimal                                       ASCII
-------------------------------------------------------------------------------------
00005633bb080000  f3 0f 1e fa 48 83 ec 08 48 8b 05 d1 4f 00 00 48  |....H...H...O..H|
00005633bb080010  85 c0 74 02 ff d0 48 83 c4 08 c3 00 00 00 00 00  |..t...H.........|
00005633bb080020  ff 35 b2 4e 00 00 f2 ff 25 b3 4e 00 00 0f 1f 00  |.5.N....%.N.....|
00005633bb080030  f3 0f 1e fa 68 00 00 00 00 f2 e9 e1 ff ff ff 90  |....h...........|
*
00005633bb0800a0  f3 0f 1e fa 68 07 00 00 00 f2 e9 71 ff ff ff 90  |....h......q....|
00005633bb0800b0  f3 0f 1e fa 68 08 00 00 00 f2 e9 61 ff ff ff 90  |....h......a....|
00005633bb0800c0  f3 0f 1e fa 68 09 00 00 00 f2 e9 51 ff ff ff 90  |....h......Q....|
00005633bb0800d0  f3 0f 1e fa 68 0a 00 00 00 f2 e9 41 ff ff ff 90  |....h......A....|
00005633bb0800e0  f3 0f 1e fa 68 0b 00 00 00 f2 e9 31 ff ff ff 90  |....h......1....|
```

Here's an example of memory analysis of a PostgreSQL container. In this case, we start a PostgreSQL container with a password set to 'mysecret'. Then, we create a checkpoint of the container and use the `memparse` to find the stored password.

```console
$ sudo podman run --name postgres -e POSTGRES_PASSWORD=mysecret -d postgres
$ sudo podman container checkpoint -l --export=/tmp/postgres.tar.gz
$ sudo checkpointctl memparse --pid 1 /tmp/postgres.tar.gz | grep -B 1 -A 1 mysecret
000055dd725c1e60  50 4f 53 54 47 52 45 53 5f 50 41 53 53 57 4f 52  |POSTGRES_PASSWOR|
000055dd725c1e70  44 3d 6d 79 73 65 63 72 65 74 00 00 00 00 00 00  |D=mysecret......|
000055dd725c1e80  00 00 00 00 00 00 00 00 31 00 00 00 00 00 00 00  |........1.......|
```

Here's another scenario, of memory analysis for a web application container. We start a vulnerable web application container, perform an arbitrary code execution attack, create a checkpoint for forensic analysis while leaving the container running, and finally analyze the checkpoint memory to identify the injected code.

```bash
# Start vulnerable web application
$ sudo podman run --name dsvw -p 1234:8000 -d quay.io/rst0git/dsvw

# Perform arbitrary code execution attack: $(echo secret)
$ curl "http://localhost:1234/?domain=www.google.com%3B%20echo%20secret"
nslookup: can't resolve '(null)': Name does not resolve

Name:      www.google.com
Address 1: 142.250.187.228 lhr25s34-in-f4.1e100.net
Address 2: 2a00:1450:4009:820::2004 lhr25s34-in-x04.1e100.net
secret

# Create a checkpoint for forensic analysis and leave the container running
$ sudo podman container checkpoint --leave-running -l -e /tmp/dsvw.tar

# Analyse checkpoint memory to identify the attacker's injected code
$ sudo checkpointctl memparse --pid 1 /tmp/dsvw.tar | grep 'echo secret'
00007faac5711f60  6f 6d 3b 20 65 63 68 6f 20 73 65 63 72 65 74 00  |om; echo secret.|
```

For larger processes, it's recommended to write the contents of process memory pages to a file rather than standard output.

To get an overview of process memory sizes within the checkpoint, run `checkpointctl memparse` without arguments.

```console
$ sudo checkpointctl memparse /tmp/jira.tar.gz

Displaying processes memory sizes from /tmp/jira.tar.gz

+-----+--------------+-------------+
| PID | PROCESS NAME | MEMORY SIZE |
+-----+--------------+-------------+
|   1 | tini         | 100.0 KiB   |
+-----+--------------+-------------+
|   2 | java         | 553.5 MiB   |
+-----+--------------+-------------+
```

In this example, given the large size of the java process, it is better to write its output to a file.

```console
$ sudo checkpointctl memparse --pid=2 /tmp/jira.tar.gz --output=/tmp/java-memory-pages.txt
Writing memory pages content for process ID 2 from checkpoint: /tmp/jira.tar.gz to file: /tmp/java-memory-pages.txt...
```

Please note that writing large memory pages to a file can take several minutes.

## Installing from source code

1. Clone the repository.

    ```console
    git clone https://github.com/checkpoint-restore/checkpointctl.git
    ```

2. Install dependencies.

    On Fedora, CentOS and related distributions:

    ```console
    sudo yum install -y go make
    ```

    On Debian, Ubuntu, and related distributions:

    ```console
    sudo apt-get install -y golang make
    ```

3. Build.

    ```console
    make
    ```

4. Once checkpointctl has been compiled, it can be installed on a system by simply typing

    ```console
    sudo make install
    ```

    This command accepts the following variables:

     * **DESTDIR**, to specify global root where all components will be placed under (empty by default);
     * **PREFIX**, to specify additional prefix for path of every component installed (`/usr/local` by default);
     * **BINDIR**, to specify where to install the checkpointctl tool (`$(PREFIX)/bin` by default);

    Thus, to install everything under `/some/new/place`, use the following command:

    ```console
    make DESTDIR=/some/new/place install
    ```

## Enable autocompletion

You now need to ensure that the autocompletion script gets sourced in all your shell sessions.
There are two ways in which you can do this:

### User

```console
echo 'source <(checkpointctl completion bash)' >>~/.bashrc
```

### System

```console
checkpointctl completion bash | sudo tee /etc/bash_completion.d/checkpointctl > /dev/null
sudo chmod a+r /etc/bash_completion.d/checkpointctl
```

Both approaches are equivalent. After reloading your shell, autocompletion should be working.
To enable bash autocompletion in current shell session, source the `~/.bashrc` file:

```console
source ~/.bashrc
```

## Uninstalling

The following command can be used to clean up a previously installed checkpointctl instance.

```console
make uninstall
```

Note that if some variable (e.g., **DESTDIR**, **BINDIR**) has been used during installation,
the same *must* be passed with uninstall action.

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
[podman]: https://podman.io/docs/checkpoint
[criu]: https://criu.org/
