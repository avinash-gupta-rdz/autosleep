package controllers

import (
	heroku "github.com/heroku/heroku-go/v5"
	"context"
	"fmt"
	"autosleep/models"
	"os"
	"strconv"
)


func add_drain(app_id string, token string)(string){
	heroku.DefaultTransport.BearerToken = token
	service := heroku.NewService(heroku.DefaultClient)
	self_url := os.Getenv("SELF_HOST")
	url := fmt.Sprintf("%s/drain/%s",self_url,app_id)
	drain, err := service.LogDrainCreate(context.TODO(), app_id, heroku.LogDrainCreateOpts{URL: url})
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println(app_id, "=>  Added drain: ", drain.ID)
	return drain.ID
}

func remove_drain(app_id string, token string, drain_id string){
	heroku.DefaultTransport.BearerToken = token
	service := heroku.NewService(heroku.DefaultClient)
	_, err := service.LogDrainDelete(context.TODO(), app_id, drain_id)
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println(app_id, "=>  Removed drain: ", drain_id)
}


func get_current_config(app_id string, token string)(map[string]map[string]string){ //TODO:: unused method

	heroku.DefaultTransport.BearerToken = token
	service := heroku.NewService(heroku.DefaultClient)
	var lrange heroku.ListRange

	formation_list, err := service.FormationList(context.TODO(), app_id, &lrange)
	if err != nil {
		fmt.Println(err)
	}
	var formation_map = map[string]map[string]string{}

	  for _, formation := range formation_list {
	    if formation.Type != "console" && formation.Type != "rake" {
	      formation_map[formation.Type] = map[string]string{}
	      formation_map[formation.Type]["Quantity"] = strconv.Itoa(formation.Quantity)
	      formation_map[formation.Type]["Size"] = formation.Size
	    }
	    
	  }
	return formation_map
}

func ScaleUpDynos(app models.Application) {
	heroku.DefaultTransport.BearerToken = app.HerokuApiKey
	service := heroku.NewService(heroku.DefaultClient)
	opts := heroku.FormationBatchUpdateOpts{}

	for f_type, formation := range app.CurrentConfig {
		qty, _ := strconv.Atoi(formation["Quantity"])
		d_size := formation["Size"]
		upd := struct {
			Quantity *int    `json:"quantity,omitempty" url:"quantity,omitempty,key"`
			Size     *string `json:"size,omitempty" url:"size,omitempty,key"`
			Type     string  `json:"type" url:"type,key"`
		}{&qty, &d_size, f_type}
		opts.Updates = append(opts.Updates, upd)
	}

	batch_result, _ := service.FormationBatchUpdate(context.TODO(), app.HerokuAppName, opts)
	fmt.Println("batch_result\n",batch_result)

}