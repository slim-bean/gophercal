# Creating the credentials for gophercal

## Todoist.com API Key

See: https://todoist.com/help/articles/find-your-api-token-Jpzx9IIlB

![Todoist Creation GIF](./images/todoist-token.gif)

## Google Calendar OAuth credentials

### Create a project

See: https://developers.google.com/workspace/guides/create-project

![Google Create Project](./images/create-google-project.png)

### Enable Calendar API

See: https://developers.google.com/workspace/guides/enable-apis

![Enable Calendar API](./images/enable-calendar-api.gif)

### Configure OAuth scopes and credentials

See: https://console.cloud.google.com/apis/api/calendar-json.googleapis.com/metrics

You need to choose `auth/userinfo.email` and `calendar.events.readonly` scope.

![OAuth Scope selection](./images/oauth-scopes.gif)

## Create client credentials

See: https://developers.google.com/workspace/guides/create-credentials

You need to select a Desktop App and download the credentials JSON file. Now save it as `credentials.json` in the folder with gophercal.

![Client Credentials](./images/create-client-credentials.gif)