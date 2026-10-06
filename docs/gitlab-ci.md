# Container builds in GitLab

The root `.gitlab-ci.yml` builds the existing production Dockerfiles in two
parallel jobs: `api` from `backend/` and `frontend` from `frontend/`.
Merge request pipelines build both images without publishing. Branch and tag
pipelines build and publish to the project's GitLab Container Registry:

```text
$CI_REGISTRY_IMAGE/api:$CI_COMMIT_SHA
$CI_REGISTRY_IMAGE/frontend:$CI_COMMIT_SHA
```

Use the same full commit SHA for both images when deploying a release. No mutable
`latest` tag is published, so concurrent pipelines cannot overwrite the selected
release. The frontend is built for the existing Compose backend address
`http://api:8080`; retain the `api` service name in the deployment network.

## GitLab setup

1. Enable Container Registry for the project (on self-managed GitLab, the
   administrator must configure the registry first).
2. Use a Docker executor runner that supports privileged Docker-in-Docker and
   shares `/certs/client` between the job and service containers. For a dedicated
   runner, the relevant `config.toml` settings are:

   ```toml
   [runners.docker]
     privileged = true
     volumes = ["/certs/client", "/cache"]
   ```

3. Allow the runner to pull the Docker, Go, Node.js and Alpine images and access
   Go modules, npm and the GitLab registry. Docker-in-Docker uses TLS on port 2376.
4. Push the repository to GitLab. No custom registry credentials are needed in
   CI: GitLab supplies `CI_REGISTRY`, `CI_REGISTRY_IMAGE`, `CI_REGISTRY_USER` and
   `CI_REGISTRY_PASSWORD`.

The pipeline only builds and publishes application images; it does not deploy
the application, run the full test suite, migrate a database or import a catalog.
The existing GitHub Actions checks remain separate.

For a future VPS deployment, create a Deploy Token with `read_registry` and use
it to pull these images. Do not copy the short-lived CI job credentials to the
server. PostgreSQL continues to use the official image and a persistent volume.
