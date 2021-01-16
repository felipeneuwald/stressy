# stressy
stressy is a simple CPU stress test tool.

It is written in Go and has amd64 [binaries](bin/) for [Linux](bin/stressy_linux), [Mac OS](bin/stressy_darwin), [FreeBSD](bin/stressy_freebsd), [NetBSD](bin/stressy_netbsd), and [OpenBSD](bin/stressy_openbsd).

## Usage

### CLI

```
./stressy -p 2 -t 300
```

Where:
- -p: Qty of parallel CPU stress tests (default 1)
- -t: Test execution time (seconds); If not specified will run indefinitely

### Docker
For the use cases where it is needed to run CPU stress tests in a containerized system, stressy provides a docker image.

```
docker run felipeneuwald/stressy ./stressy_linux -p 8 -t 1800
```

In the example above, stressy runs 8 parallel CPU tests for 1800 seconds (30 minutes).

### Kubernetes
Stressy can run in a Kubernetes cluster for application performance tests.

The example below shows how to create a Kubernetes job that will run stressy for 86400 seconds (24 hours): 

```
kubectl create job stressy --image=felipeneuwald/stressy -- ./stressy_linux -t 86400
```

## License

See [LICENSE](LICENSE).