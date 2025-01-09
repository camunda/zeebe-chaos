# Zeebe Chaos deployment

The zbchaos worker is deployed via the [deployment.yaml](/go-chaos/deploy/deployment.yaml) file to the Zeebe Team GKE (zeebe-io). It will connect to the Testbench production environment (Zeebe cluster in SaaS) and poll for work.

If there is a new chaos experiment executed (and a target cluster created in a separate chaos GKE) zbchaos will connect to such cluster and run certain actions, like verifications or chaos injections. In order to connect to such chaos cluster zbchaos uses a separate kubeconfig, which is [injected via a secret](https://github.com/camunda/zeebe-chaos/blob/main/go-chaos/deploy/deployment.yaml#L60). 

The secret is currently deployed **manually** but contains the content of the [kubernetes config file stored in this repository](https://github.com/camunda/zeebe-chaos/blob/main/go-chaos/deploy/kubeconfig.yaml).

## Kubernetes configuration file

The Kubernetes configuration file is encrypted using [sops](https://github.com/mozilla/sops).


You need the right permissions to decrypt or encrypt the file. We have created in our [google cloud project a key ring set up](https://github.com/camunda/zeebe-chaos/blob/main/go-chaos/deploy/kubeconfig.yaml). The Zeebe and SRE team should have access to they key-ring (and keys). 


If there are any updates necessary on the config, SREs can easily update the config in this repository. We should make sure to reflect the changes in our testbench namespace.


### Accessing the Kube config
To make changes to the file use for example:

```
sops kubeconfig.yaml
```

  - The file will be decrypted and opened in your favorite editor.
  - It will be automatically re-encrypted when the editor is closed.


To decrypt the encrypted file in-place, run:

```
  sops --decrypt --in-place kubeconfig.yaml
```

To encrypt the decrypted file in-place, run:

```
  sops --encrypt --in-place kubeconfig.yaml
```

For more information, take a look at [the official documentation](https://github.com/mozilla/sops).
