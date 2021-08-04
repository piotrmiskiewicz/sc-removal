# sc-removal
This repository holds code for Service Management removal.

## Build and push the image

```shell
# authorize and configure gcloud
gcloud auth login
gcloud auth configure-docker

# build and push
SSH_PRIVATE_KEY="$(cat ~/.ssh/id_rsa)" make build-image push-image
```

