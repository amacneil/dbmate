# Google BigQuery Test Fixtures

## Creating a service account for testing

From the `dbmate` top-level directory:

```sh
$ PROJECT_ID=your-google-cloud-project-id
$ LOCATION=us-east5
$ DATASET=test_dataset
$ SERVICE_ACCOUNT=dbmate-test-sa

$ gcloud auth login

$ gcloud iam service-accounts create $SERVICE_ACCOUNT

$ gcloud projects add-iam-policy-binding $PROJECT_ID \
  --role="roles/bigquery.dataEditor" \
  --member=serviceAccount:${SERVICE_ACCOUNT}@${PROJECT_ID}.iam.gserviceaccount.com

$ gcloud projects add-iam-policy-binding $PROJECT_ID \
  --role="roles/bigquery.jobUser" \
  --member=serviceAccount:${SERVICE_ACCOUNT}@${PROJECT_ID}.iam.gserviceaccount.com

$ gcloud iam service-accounts keys create \
  fixtures/bigquery/credentials.json \
  --iam-account=${SERVICE_ACCOUNT}@${PROJECT_ID}.iam.gserviceaccount.com

## WARNING: Only do this on a private machine, as anyone else with
## access to the system will also be able to read the credentials file's
## contents once it's made world-readable and use it to access Google as
## the service account.  This is necessary for dbmate running as root
## inside the Docker container to be able to read the file, though.

$ chmod a+r fixtures/bigquery/credentials.json

$ docker compose run --rm dev

## The rest of these commands should be executed from inside the Docker
## container:

$ make build

$ make test \
  GOOGLE_APPLICATION_CREDENTIALS=/src/fixtures/bigquery/credentials.json \
  GOOGLE_BIGQUERY_TEST_URL=bigquery://$PROJECT_ID/$LOCATION/$DATASET \
  FLAGS+="-count 1 -v ./pkg/driver/bigquery #"

```
