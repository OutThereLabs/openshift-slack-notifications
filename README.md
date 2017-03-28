# OpenShift Slack Notifications

A project to send OpenShift error messages to a slack channel of your choice.

## Cluster Requirements

First you need a running minishift cluster. This can be installed via homebrew:

```shell
$ brew install socat openshift-cli docker-machine-driver-xhyve
$ brew tap caskroom/versions
$ brew cask install minishift-beta
```

The xhyve hypervisor requires superuser privileges. To enable, execute:

```shell
$ sudo chown root:wheel /usr/local/opt/docker-machine-driver-xhyve/bin/docker-machine-driver-xhyve
$ sudo chmod u+s /usr/local/opt/docker-machine-driver-xhyve/bin/docker-machine-driver-xhyve
```

Then start the cluster with:

```shell
$ minishift start --memory 4048
```

## App Requirements

First add the privileges to mount volumes and read cluster state to your service account:

```shell
$ oc login -u system:admin
$ oc adm policy add-scc-to-user hostmount-anyuid system:serviceaccount:myproject:default --as=system:admin
$ oc adm policy add-cluster-role-to-user cluster-reader system:serviceaccount:myproject:default --as=system:admin
```

Then create your dev environment via the provided template

```shell
$ oc process -f template.yaml -v SOURCE_PATH="${PWD}" SLACK_WEBHOOK_URL=<slack-webhook-url-here> OPENSHIFT_CONSOLE_URL=https://<cluster-ip-here>:8443/console | oc create -f -
$ oc start-build go
```

## Running the application

To run a local copy, start a debug pod

```shell
$ oc debug dc/go-dev
$ cd go/github.com/outtherelabs/openshift-slack-notifications && glide up
$ go run src/main.go
```
