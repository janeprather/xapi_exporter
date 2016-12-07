#!/bin/sh
#
# this script will attempt to build the static binary of xapi_exporter
# and then install it into a more space-conscious container
#

if [ -z "${1}" ]
then
  echo "This script takes one argument: the tag name for the final container"
  echo ""
  echo "Example: ${0} xapi_exporter"
  exit 1
fi

docker build -t xapi_exporter_staticbuild -f Dockerfile.build .

CONTAINER_ID=`docker run -d -t xapi_exporter_staticbuild`
if [ -z "${CONTAINER_ID}" ]
then
  echo "Failed to get static build container id" >&2
  exit 1
fi

docker cp ${CONTAINER_ID}:/bin/xapi_exporter ./xapi_exporter
if [ $? != 0 ]
then
  echo "Failed to cp /bin/xapi_exporter out of static build container" >&2
  exit 1
fi

docker stop ${CONTAINER_ID}
if [ $? != 0 ]
then
  echo "Failed to stop static build container" >&2
  exit 1
fi

docker rm ${CONTAINER_ID}
if [ $? != 0 ]
then
  echo "Failed to remove static build container" >&2
  exit 1
fi

IMAGE_ID=`docker images -q xapi_exporter_staticbuild`
if [ -z "${IMAGE_ID}" ]
then
  echo "Failed to determine static build image id" >&2
  exit 1
fi

docker rmi ${IMAGE_ID}
if [ $? != 0 ]
then
  echo "Failed to remove static build image" >&2
  exit 1
fi

docker build -t "${1}" -f Dockerfile.run .
if [ $? != 0 ]
then
  echo "Failed to build run image" >&2
  exit 1
fi

rm xapi_exporter
