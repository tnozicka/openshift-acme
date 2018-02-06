#!/bin/bash
set -e

script_full_path=$(readlink -f $0)
script_dir=$(dirname ${script_full_path})
pushd ${script_dir}/.. 1>/dev/null

outdir=${1:-$(mktemp -d)}

cw_common_files=$(find ./deploy/letsencrypt-staging/cluster-wide -type f -not -path ./deploy/letsencrypt-staging/cluster-wide/clusterrole.yaml -printf '%P ')
sn_common_files=$(find ./deploy/letsencrypt-staging/single-namespace -type f -not -path ./deploy/letsencrypt-staging/single-namespace/role.yaml -printf '%P ')
diff <(echo "${cw_common_files}") <(echo "${sn_common_files}")

for file in ${cw_common_files}; do
    diff deploy/letsencrypt-staging/cluster-wide/${file} deploy/letsencrypt-staging/single-namespace/${file} > ${outdir}/staging-${file}.diff || [[ "$?" == 1 ]]
done

diff deploy/letsencrypt-staging/cluster-wide/clusterrole.yaml deploy/letsencrypt-staging/single-namespace/role.yaml > ${outdir}/staging-roles.diff || [[ $? == 1 ]]

diff deploy/letsencrypt-live/cluster-wide/ deploy/letsencrypt-staging/cluster-wide/ > ${outdir}/live-staging-cluster-wide.diff || [[ $? == 1 ]]

diff deploy/letsencrypt-live/single-namespace/ deploy/letsencrypt-staging/single-namespace/ > ${outdir}/live-staging-single-namespace.diff || [[ $? == 1 ]]


diff -r deploy/.diffs ${outdir}
