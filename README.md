AutoSleep 
-------------------------------------------------
Sleep your dynos when you are sleeping

This is an OpenSource Project inspired by the paid Tool [AutoIdle](https://autoidle.com/)

It uses heroku syslog drains to detect if any router log is generated, if router logs are availabel in the logs it assume application is running

## Language & Framework

AutoSleep is written in golang,
Api's are implimented using [GIN](https://gin-gonic.com/) and 
Worker Backgroud Job uses [work](https://github.com/gocraft/work)


## API DOC

[API DOC](https://github.com/avinash-gupta-rdz/autosleep/blob/master/api_doc.md)

## Configurations maintained with ENV

| ENV Variable     | usage |
|------------------|--------|
| DATABASE_URL  | Mysql Database URI |
| REDISCLOUD_URL |Redis URI |
| PASSPHRASE |used to generate encryption key |
| SELF_HOST | self-hosted URL used to consume log drains |
| API_USER |Basic Auth User |
| API_PASS |Basic Auth Password |


### TODO:
- Maintain History
- Calculate Saving $
- Build UI
- Get Code Reviewed


