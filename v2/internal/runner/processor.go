package runner

import (
	"github.com/projectdiscovery/gologger"
	"github.com/projectdiscovery/mapsutil"
	"github.com/projectdiscovery/nuclei/v2/pkg/output"
	"github.com/projectdiscovery/nuclei/v2/pkg/protocols/common/starlight"
	"github.com/projectdiscovery/nuclei/v2/pkg/templates"
	"github.com/remeh/sizedwaitgroup"
	"go.uber.org/atomic"
)

// processTemplateWithList process a template on the URL list
func (r *Runner) processTemplateWithList(template *templates.Template) bool {
	results := &atomic.Bool{}
	wg := sizedwaitgroup.New(r.options.BulkSize)
	r.hostMap.Scan(func(k, _ []byte) error {
		URL := string(k)

		wg.Add()
		go func(URL string) {
			defer wg.Done()

			match, err := template.Executer.Execute(URL)
			if err != nil {
				gologger.Warning().Msgf("[%s] Could not execute step: %s\n", r.colorizer.BrightBlue(template.ID), err)
			}
			results.CAS(false, match)
		}(URL)
		return nil
	})
	wg.Wait()
	return results.Load()
}

// processTemplateWithList process a template on the URL list
func (r *Runner) processWorkflowWithList(template *templates.Template) bool {
	results := &atomic.Bool{}
	wg := sizedwaitgroup.New(r.options.BulkSize)

	r.hostMap.Scan(func(k, _ []byte) error {
		URL := string(k)
		wg.Add()
		go func(URL string) {
			defer wg.Done()
			match := template.CompiledWorkflow.RunWorkflow(URL)
			results.CAS(false, match)
		}(URL)
		return nil
	})
	wg.Wait()
	return results.Load()
}

// processTemplateWithList process a template on the URL list
func (r *Runner) processAdvancedWorkflowWithList(template *templates.Template) bool {
	results := &atomic.Bool{}
	wg := sizedwaitgroup.New(r.options.BulkSize)

	r.hostMap.Scan(func(k, _ []byte) error {
		URL := string(k)
		wg.Add()
		go func(URL string) {
			defer wg.Done()
			match := r.RunAdvancedWorkflow(template, URL)
			results.CAS(false, match)
		}(URL)
		return nil
	})
	wg.Wait()
	return results.Load()
}

// RunWorkflow runs a workflow on an input and returns true or false
func (r *Runner) RunAdvancedWorkflow(template *templates.Template, input string) bool {
	// run templates as callable function
	vars := make(map[string]interface{})
	runWithValues := func(templateName string, args map[interface{}]interface{}) map[interface{}]interface{} {
		// get full template path
		tpath := r.catalog.GetTemplatesPath([]string{templateName})
		if len(tpath) == 0 {
			gologger.Fatal().Msgf("Could not parse file '%s'\n", templateName)
		}
		t, err := r.parseTemplateFile(tpath[0])
		if err != nil {
			gologger.Fatal().Msgf("Could not parse file '%s': %s\n", templateName, err)
		}
		res, err := processTemplateWithResults(args["URL"].(string), t)
		if err != nil {
			gologger.Fatal().Msgf("%s", err)
		}
		return res
	}
	vars["run_with_values"] = runWithValues
	vars["run"] = func(template string, args map[interface{}]interface{}) bool {
		d := runWithValues(template, args)
		if v, ok := d["matched"].(bool); ok {
			return v
		}
		return false
	}

	vars["URL"] = input
	res, err := starlight.ExecScript(template.Code, vars)
	if err != nil {
		gologger.Fatal().Msgf("%s", err)
	}

	return res != nil
}

func processTemplateWithResults(URL string, template *templates.Template) (map[interface{}]interface{}, error) {
	results := make(map[string]interface{})
	err := template.Executer.ExecuteWithResults(URL, func(result *output.InternalWrappedEvent) {
		results = mapsutil.MergeMaps(results, result.OperatorsResult.DynamicValues)
		results = mapsutil.MergeMaps(results, result.OperatorsResult.PayloadValues)
		for k, v := range result.OperatorsResult.Extracts {
			results[k] = v
		}
		for k, v := range result.OperatorsResult.Matches {
			results[k] = v
		}
		results["extracted"] = result.OperatorsResult.Extracted
		results["matched"] = result.OperatorsResult.Matched
		results["output_extracts"] = result.OperatorsResult.OutputExtracts

	})
	if err != nil {
		return nil, err
	}

	cresults := make(map[interface{}]interface{})
	for k, v := range results {
		cresults[k] = v
	}

	return cresults, nil
}
