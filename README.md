To build and run the k8s-e2e-collector, run this command:

```
cd k8s-tests
go build -o bin/k8s-e2e-collector ./cmd/graph-collector
cd bin
chmod +x k8s-e2e-collector
./k8s-e2e-collector
```