# stressy
stressy is a simple CPU stress test tool.

It is written in Go and has amd64 binaries for Linux, Mac OS, FreeBSD, NetBSD and OpenBSD.

```
$ stressy -help
Usage of stressy:
  -parallelism int
    	Number of parallel operations (default 1)
  -time int
    	Number of seconds to run (default 86400)
```

## Docker
```
docker run felipeneuwald/stressy ./stressy_linux -p 2 -t 10
```
