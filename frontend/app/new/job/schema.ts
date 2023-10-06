import { getAccount } from '@/components/providers/account-provider';
import { IsJobNameAvailableResponse } from '@/neosync-api-client/mgmt/v1alpha1/job_pb';
import {
  DESTINATION_FORM_SCHEMA,
  SCHEMA_FORM_SCHEMA,
  SOURCE_FORM_SCHEMA,
} from '@/yup-validations/jobs';
import cron from 'cron-validate';
import * as Yup from 'yup';

export const DEFINE_FORM_SCHEMA = Yup.object({
  jobName: Yup.string()
    .trim()
    .required('Name is a required field')
    .min(3)
    .max(30)
    .test('checkNameUnique', 'This name is already taken.', async (value) => {
      if (!value || value.length == 0) {
        return false;
      }
      const account = getAccount();
      if (!account) {
        return false;
      }
      const res = await isJobNameAvailable(value, account.id);
      return res.isAvailable;
    }),
  cronSchedule: Yup.string()
    .optional()
    .test('isValidCron', 'Not a valid cron schedule', (value) => {
      if (!value) {
        return true;
      }
      return !!value && cron(value).isValid();
    }),
});

export type DefineFormValues = Yup.InferType<typeof DEFINE_FORM_SCHEMA>;

export const FLOW_FORM_SCHEMA = SOURCE_FORM_SCHEMA.concat(
  Yup.object({ destinations: Yup.array(DESTINATION_FORM_SCHEMA).required() })
);
export type FlowFormValues = Yup.InferType<typeof FLOW_FORM_SCHEMA>;

const FORM_SCHEMA = Yup.object({
  define: DEFINE_FORM_SCHEMA,
  flow: FLOW_FORM_SCHEMA,
  schema: SCHEMA_FORM_SCHEMA,
});

export type FormValues = Yup.InferType<typeof FORM_SCHEMA>;

async function isJobNameAvailable(
  name: string,
  accountId: string
): Promise<IsJobNameAvailableResponse> {
  const res = await fetch(
    `/api/jobs/is-job-name-available?name=${name}&accountId=${accountId}`,
    {
      method: 'GET',
      headers: {
        'content-type': 'application/json',
      },
    }
  );
  if (!res.ok) {
    const body = await res.json();
    throw new Error(body.message);
  }
  return IsJobNameAvailableResponse.fromJson(await res.json());
}
