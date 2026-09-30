#!/usr/bin/env bash
set -e

docker login -u cofco-cn-beijing-1_liangdawang@QTK3P4F907W9JLX13UIL -p 8e3fccaa14aa459ac2b0675b93674d5586770ebbec575b58880b3be022d78603 https://swr.cofco-cn-beijing-1.cofco.com

gitTag=$(git rev-parse --short HEAD)
buildDate=$(date +%Y%m%d%H%M%S)
imageTag="${buildDate}-${gitTag}"
imageName="swr.cofco-cn-beijing-1.cofco.com/liangdawang-test-liangdawang/stg/ldw-new-api:$imageTag"
docker build -f xc_env/Dockerfile -t $imageName .
docker push $imageName
docker rmi $imageName


# 定义 K8s 变量（与 xc_env/stg.yaml 保持一致）
NAMESPACE="server"                     # 命名空间
DEPLOYMENT_NAME="ldw-new-api"          # Deployment 名称

echo "开始部署 Kubernetes 服务: $DEPLOYMENT_NAME in namespace $NAMESPACE"

# 把 xc_env/stg.yaml 里的镜像替换成本次构建的真实 tag（避免占位 latest 拉不到镜像）
sed -i "s#swr.cofco-cn-beijing-1.cofco.com/liangdawang-test-liangdawang/stg/ldw-new-api:.*#swr.cofco-cn-beijing-1.cofco.com/liangdawang-test-liangdawang/stg/ldw-new-api:${imageTag}#g" xc_env/stg.yaml

# 应用清单：首次创建 Deployment/Service，已存在则滚动更新；apply 的就是最新镜像
kubectl apply -f xc_env/stg.yaml -n ${NAMESPACE}

# 监控滚动更新状态（等待 Pod 启动完成，失败则 Jenkins 红灯）
kubectl rollout status deployment/${DEPLOYMENT_NAME} -n ${NAMESPACE}

echo "部署完成！当前运行的镜像为: $imageName"
exit
