# `generate` Command

mlp generate

mlp generate -filename=[] --env-prefix=[]
mlp interpolate --filename=./console.yaml --env-prefix=DEV_ --env-prefix=MIA_

{{[A-Z0-9_]+}}

```
config-map:
  - name: "pippo"
    data:
      - from: "literal|file"
        file: ./path
        key: key
        value: value
      - from: literal
        key: key1
        value: value1
      - from: literal
        key: key2
        value: {{PIPPO}}

secret:
  - name: "pippo"
    when: "always|once"
    type: "generic|tls|docker-registry"
    tls:
      cert: path
      key: path
    docker:
      username: {{DOCKER_USERNAME}}
      password: {{DOCKER_USERNAME}}
      email: {{DOCKER_USERNAME}}
      server: {{DOCKER_USERNAME}}
    data:
      - from: "literal|file"
        file: ./path
        key: key
        value: {{mlp}}
```
