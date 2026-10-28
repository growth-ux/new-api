#!/usr/bin/env bash
set -euo pipefail

# Jenkins 设置 DEPLOY_ENV=stg/prod、DEPLOY_REMOTE=部署用户@跳板机。
# Harbor 凭据通过 Jenkins 注入 HARBOR_USERNAME、HARBOR_PASSWORD；也可提前登录。
DEPLOY_ENV="${DEPLOY_ENV:-stg}"
case "$DEPLOY_ENV" in
  stg|prod) ;;
  *) echo "DEPLOY_ENV 只支持 stg 或 prod" >&2; exit 1 ;;
esac
: "${DEPLOY_REMOTE:?请设置 DEPLOY_REMOTE 为执行 kubectl 的跳板机地址}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
cd "$PROJECT_ROOT"
manifest="$SCRIPT_DIR/$DEPLOY_ENV.yaml"

if ! awk '$1 !~ /^#/ && /replace-with-/ {exit 1}' "$manifest"; then
  echo "请先填写 $manifest 中的生产环境占位值" >&2
  exit 1
fi

# 从清单读取资源名称和 namespace，避免脚本变量与实际资源不一致。
namespace="$(awk '$1 == "namespace:" {print $2; exit}' "$manifest")"
deployment="$(awk '$1 == "name:" {print $2; exit}' "$manifest")"
template_image="$(awk '$1 == "image:" {print $2; exit}' "$manifest")"
image_repository="${template_image%:*}"
if [[ ! "$namespace" =~ ^[a-z0-9][a-z0-9-]*$ || ! "$deployment" =~ ^[a-z0-9][a-z0-9.-]*$ ]]; then
  echo "清单中的 namespace 或 Deployment 名称无效" >&2
  exit 1
fi
if [[ "$(awk '$1 == "namespace:" {print $2}' "$manifest" | sort -u)" != "$namespace" ]]; then
  echo "Deployment 和 Service 的 namespace 必须一致" >&2
  exit 1
fi

if [[ -n "${HARBOR_USERNAME:-}" ]]; then
  : "${HARBOR_PASSWORD:?请通过 Jenkins Credentials 注入 HARBOR_PASSWORD}"
  set +x
  printf '%s' "$HARBOR_PASSWORD" | docker login "${image_repository%%/*}" \
    --username "$HARBOR_USERNAME" --password-stdin
fi

image_tag="$(date +%Y%m%d%H%M%S)-$(git rev-parse --short HEAD)-${BUILD_NUMBER:-local}"
image_name="$image_repository:$image_tag"
docker build --platform linux/amd64 -f "$SCRIPT_DIR/Dockerfile" -t "$image_name" .
docker push "$image_name"

# 渲染临时清单，不修改仓库中的 YAML 模板。
rendered_manifest="$(mktemp)"
trap 'rm -f "$rendered_manifest"' EXIT
sed -E "s#^([[:space:]]*image:[[:space:]]*).*#\1$image_name#" "$manifest" > "$rendered_manifest"
remote_manifest="/tmp/ldw-new-api-online-${rendered_manifest##*/}.yaml"
scp "$rendered_manifest" "$DEPLOY_REMOTE:$remote_manifest"

ssh "$DEPLOY_REMOTE" bash -s -- "$namespace" "$deployment" "$remote_manifest" <<'REMOTE_SCRIPT'
set -euo pipefail
namespace="$1"
deployment="$2"
manifest="$3"
trap 'rm -f "$manifest"' EXIT
/usr/bin/kubectl apply -f "$manifest" -n "$namespace"
/usr/bin/kubectl rollout status "deployment/$deployment" -n "$namespace" --timeout=600s
REMOTE_SCRIPT

echo "部署完成：${DEPLOY_ENV} / ${namespace} / ${deployment}，镜像：${image_name}"
