import { request } from '@/service/http'

/**
 * Query FASTQ files from GSF
 * @param {Object} params
 * @param {string} params.pigrp
 * @param {string} params.soid
 * @param {string} params.organism
 * @param {string} params.reference
 * @param {string} params.assay
 * @param {string} params.seqtype
 */
export function fetchFastqQuery(params) {
  const method = request.Get('/cab/fastqQuery', { params })
  method.meta = { isRawResponse: true }
  return method
}

/**
 * Submit job to HPC
 * @param {Object} data
 */
export function fetchSubmitJob(data) {
  const method = request.Post('/cab/pipeline', data)
  method.meta = { isRawResponse: true }
  return method
}

