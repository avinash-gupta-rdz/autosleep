AutoSleep API Documentation
----------------------------

`:app_id` is your Heroku app name
`heroku_api_key` can be retrieved with 
`heroku authorizations:create`
`manual_mode` if true then your dynos will not Scaled-up when a request comes (default: false)
`night_mode` if true than dynos will only Scaled-down in Night i.e., 9 PM IST to Next 12 hours (default: false)
`check_interval` After what interval scheduler will keep checking for dyno sleep (default: 600  seconds)
`ideal_time`  Ideal Time duration after which the dynos will scaled-down if not in use(default: 1800 seconds)


Note: 
1) check_interval should be less then ideal_time otherwise ideal_time will be equal to check_interval
2) ideal_time should not vary small otherwise it can disturb user experience 

see: [heroku authentication](https://devcenter.heroku.com/articles/platform-api-quickstart#authentication) 

### Add App for autosleep

```sh
curl --location --request POST 'https://<YOUR_DOMAIN_NAME>/app' \
--header 'Content-Type: application/json' \
--data-raw '{
	"heroku_app_name":"heroku_app_name",
	"ideal_time": 300,
	"check_interval": 100,
	"manual_mode":false,
	"night_mode",false,
	"heroku_api_key":"<your API key>"
}'
```

### GET App Details 

```sh
curl --location --request GET 'https://<YOUR_DOMAIN_NAME>/apps/:app_id'
```

### Remove App from autoscale

```sh
curl --location --request DELETE 'https://<YOUR_DOMAIN_NAME>/apps/:app_id'
```

### Find All Apps configured for Sleep

```sh
curl --location --request GET 'https://<YOUR_DOMAIN_NAME>/apps'
```

### Consume Syslog drains

```sh
curl --location --request POST 'https://<YOUR_DOMAIN_NAME>/drain/:app_id' \
--header 'Logplex-Msg-Count: 2' \
--header 'Logplex-Drain-Token: sdsssdsdasd' \
--header 'Content-Type: application/logplex-1' \
--header 'Cookie: ahoy_visitor=b46259d7-578e-4083-8c61-8c7909eb4822' \
--data-raw '83 <40>1 2012-11-30T06:45:29+00:00 host router www.3 - State changed from starting to up
119 <40>1 2012-11-30T06:45:26+00:00 host app web.3 - Starting process with command `bundle exec rackup config.ru -p 24405`'
```

