AutoSleep API Documentation
----------------------------

`:app_id` is your heroku app name
`heroku_api_key` can be retrieved with 
`heroku authorizations:create`

see: [heroku authentication](https://devcenter.heroku.com/articles/platform-api-quickstart#authentication) 

### Add App for autosleep

```
curl --location --request POST 'https://<YOUR_DOMAIN_NAME>/app' \
--header 'Content-Type: application/json' \
--data-raw '{
	"heroku_app_name":"heroku_app_name",
	"include_worker":true,
	"heroku_api_key":"<your API key>"
}'
```

### GET App Details 

```
curl --location --request GET 'https://<YOUR_DOMAIN_NAME>/apps/:app_id'
```

### Remove App from autoscale

```
curl --location --request DELETE 'https://<YOUR_DOMAIN_NAME>/apps/:app_id'
```

### Find All Apps configured for Sleep

```
curl --location --request GET 'https://<YOUR_DOMAIN_NAME>/apps'
```

### Consume Syslog drains

```
curl --location --request POST 'https://<YOUR_DOMAIN_NAME>/drain/:app_id' \
--header 'Logplex-Msg-Count: 2' \
--header 'Logplex-Drain-Token: sdsssdsdasd' \
--header 'Content-Type: application/logplex-1' \
--header 'Cookie: ahoy_visitor=b46259d7-578e-4083-8c61-8c7909eb4822' \
--data-raw '83 <40>1 2012-11-30T06:45:29+00:00 host router www.3 - State changed from starting to up
119 <40>1 2012-11-30T06:45:26+00:00 host app web.3 - Starting process with command `bundle exec rackup config.ru -p 24405`'
```

