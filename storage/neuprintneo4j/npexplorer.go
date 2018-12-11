package neuprintneo4j

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/connectome-neuprint/neuPrintHTTP/api/npexplorer"
)

const (
	NeuronMetaQuery = "MATCH (n :`{dataset}-Neuron`) UNWIND KEYS(n) AS x RETURN DISTINCT x AS pname"

	NeuronMetaValsQuery = "MATCH (n :`{dataset}-Neuron`) RETURN DISTINCT n.{metakey} AS val"

	ROIQuery = "MATCH (neuron :`{dataset}-Neuron`) RETURN neuron.bodyId AS bodyid, neuron.roiInfo AS roiInfo"

	AutapsesQuery = "MATCH (n:`{dataset}-Neuron`)-[x:ConnectsTo]->(n) RETURN n.bodyId AS id, x.weight AS weight, n.name AS name ORDER BY x.weight DESC"

	IntersectingROIQuery = "MATCH (neuron :`{dataset}-Neuron`) WHERE neuron.{neuronid} RETURN neuron.bodyId AS bodyid, neuron.name AS bodyname, neuron.roiInfo AS roiInfo ORDER BY neuron.bodyId"

	SimpleConnectionsQuery = "MATCH (m:`{dataset}-Neuron`){connection}(n) WHERE m.{neuronid} RETURN m.name AS Neuron1, n.name AS Neuron2, n.bodyId AS Neuron2Id, e.weight AS Weight, m.bodyId AS Neuron1Id ORDER BY m.name, m.bodyId, e.weight DESC"

	RankedTableQuery  = "MATCH (m:`{dataset}-Neuron`)-[e:ConnectsTo]-(n) WHERE m.{neuronid} RETURN m.name AS Neuron1, n.name AS Neuron2, e.weight AS Weight, n.bodyId AS Body2, m.neuronType AS Neuron1Type, n.type AS Neuron2Type, id(m) AS m_id, id(n) AS n_id, id(startNode(e)) AS pre_id, m.bodyId AS Body1 ORDER BY m.bodyId, e.weight DESC"
	DistributionQuery = "MATCH (n:`{dataset}-Segment` {`{ROI}`: true}) {preorpost_filter} WITH n.bodyId as bodyId, apoc.convert.fromJsonMap(n.roiInfo)[\"{ROI}\"].{preorpost} AS {preorpost}size WHERE {preorpost}size > 0 WITH collect({id: bodyId, {preorpost}: {preorpost}size}) as bodyinfoarr, sum({preorpost}size) AS tot UNWIND bodyinfoarr AS bodyinfo RETURN bodyinfo.id AS id, bodyinfo.{preorpost} AS size, tot AS total ORDER BY bodyinfo.{preorpost} DESC"

	CompletenessQuery = "MATCH (n:`{dataset}-{NeuronSegment}`) {has_conditions} {pre_cond} {post_cond} {status_conds} WITH apoc.convert.fromJsonMap(n.roiInfo) AS roiInfo WITH roiInfo AS roiInfo, keys(roiInfo) AS roiList UNWIND roiList AS roiName WITH roiName AS roiName, sum(roiInfo[roiName].pre) AS pre, sum(roiInfo[roiName].post) AS post MATCH (meta:Meta:{dataset}) WITH apoc.convert.fromJsonMap(meta.roiInfo) AS globInfo, roiName AS roiName, pre AS pre, post AS post RETURN roiName AS unlabelres, pre AS roipre, post AS roipost, globInfo[roiName].pre AS totalpre, globInfo[roiName].post AS totalpost ORDER BY roiName"

	CommonConnectivityQuery = "WITH [{neuron_list}] AS queriedNeurons MATCH (k:`{dataset}-{NeuronSegment}`){connection}(c) WHERE (k.{idorname} IN queriedNeurons {pre_cond} {post_cond} {status_conds}) WITH k, c, r, toString(k.{idorname})+\"_weight\" AS dynamicWeight RETURN collect(apoc.map.fromValues([\"{inputoroutput}\", c.bodyId, \"name\", c.name, dynamicWeight, r.weight])) AS map"

	FindNeuronsQuery = " MATCH (m:Meta{dataset:'{dataset}'}) WITH m.superLevelRois AS rois MATCH (neuron :`{dataset}-{NeuronSegment}`) {has_conditions} {hasneuron}{neuronid} {pre_cond} {post_cond} {roi_list} RETURN neuron.bodyId AS bodyid, neuron.name AS bodyname, neuron.status AS neuronStatus, neuron.roiInfo AS roiInfo, neuron.size AS size, neuron.pre AS npre, neuron.post AS npost, rois, exists((neuron)-[:Contains]->(:Skeleton)) AS hasSkeleton ORDER BY neuron.bodyId"
)

// ExplorerFindNeurons implements API to find neurons in a certain ROI
func (store Store) ExplorerFindNeurons(params npexplorer.FindNeuronsParams) (res interface{}, err error) {
	cypher := strings.Replace(FindNeuronsQuery, "{dataset}", params.Dataset, -1)

	initcond := false
	cypher, err2 := subName(params.NeuronName, params.NeuronId, cypher)
	// if name exists, then add where statement
	if err2 == nil {
		initcond = true
		cypher = strings.Replace(cypher, "{hasneuron}", "neuron.", -1)
	} else {
		cypher = strings.Replace(cypher, "{hasneuron}", "", -1)
	}

	if params.AllSegments {
		cypher = strings.Replace(cypher, "{NeuronSegment}", "Segment", -1)
	} else {
		cypher = strings.Replace(cypher, "{NeuronSegment}", "Neuron", -1)
	}

	if params.PreThreshold > 0 {
		prestr := "(neuron.pre >= " + strconv.Itoa(params.PreThreshold) + ")"
		if initcond {
			prestr = "AND " + prestr
		}
		cypher = strings.Replace(cypher, "{pre_cond}", prestr, -1)
		initcond = true
	} else {
		cypher = strings.Replace(cypher, "{pre_cond}", "", -1)
	}

	if params.PostThreshold > 0 {
		poststr := "(neuron.post >= " + strconv.Itoa(params.PostThreshold) + ")"
		if initcond {
			poststr = "AND " + poststr
		}
		initcond = true
		cypher = strings.Replace(cypher, "{post_cond}", poststr, -1)
	} else {
		cypher = strings.Replace(cypher, "{post_cond}", "", -1)
	}

	statusarr := ""
	for index, status := range params.Statuses {
		if index == 0 {
			if initcond {
				statusarr = statusarr + "AND "
			}
			statusarr = statusarr + "("
		} else {
			statusarr = statusarr + " OR "
		}

		statusarr = statusarr + "neuron.status = \"" + status + "\""
	}
	if statusarr != "" {
		initcond = true
		statusarr = statusarr + ")"
	}
	cypher = strings.Replace(cypher, "{status_conds}", statusarr, -1)

	roilist := ""
	for index, roi := range params.InputROIs {
		if initcond && index == 0 {
			roilist += " AND "
		} else if index > 0 {
			roilist += " AND "
		}
		roilist = roilist + "(neuron.`" + roi + "`= true)"
		initcond = true
	}
	for index, roi := range params.OutputROIs {
		if initcond && index == 0 {
			roilist += " AND "
		} else if index > 0 {
			roilist += " AND "
		}
		roilist = roilist + "(neuron.`" + roi + "`= true)"
		initcond = true
	}

	cypher = strings.Replace(cypher, "{roi_list}", roilist, -1)

	if initcond {
		cypher = strings.Replace(cypher, "{has_conditions}", "WHERE", -1)
	} else {
		cypher = strings.Replace(cypher, "{has_conditions}", "", -1)
	}

	return store.makeRequest(cypher)
}

// ExplorerNeuronMetaVals implements API to find distinct values for a given meta key stored for the dataset
func (store Store) ExplorerNeuronMetaVals(params npexplorer.MetaValParams) (res interface{}, err error) {

	cypher := strings.Replace(NeuronMetaValsQuery, "{dataset}", params.Dataset, -1)
	cypher = strings.Replace(cypher, "{metakey}", params.KeyName, -1)
	return store.makeRequest(cypher)
}

// ExplorerNeuronMeta implements API to find meta information stored for the dataset
func (store Store) ExplorerNeuronMeta(params npexplorer.DatasetParams) (res interface{}, err error) {

	cypher := strings.Replace(NeuronMetaQuery, "{dataset}", params.Dataset, -1)
	return store.makeRequest(cypher)
}

// ExplorerROIConnectivity implements API to find how ROIs are connected
func (store Store) ExplorerROIConnectivity(params npexplorer.DatasetParams) (res interface{}, err error) {
	cypher := strings.Replace(ROIQuery, "{dataset}", params.Dataset, -1)
	return store.makeRequest(cypher)
}

// ExplorerRankedTable implements API to show connectivity broken down by cell type
func (store Store) ExplorerRankedTable(params npexplorer.ConnectionsParams) (res interface{}, err error) {
	cypher := strings.Replace(RankedTableQuery, "{dataset}", params.Dataset, -1)
	cypher, err = subName(params.NeuronName, params.NeuronId, cypher)
	if err != nil {
		return
	}
	return store.makeRequest(cypher)
}

// ExplorerSimpleConnections implements API to show connectivity for a give neuron
func (store Store) ExplorerSimpleConnections(params npexplorer.ConnectionsParams) (res interface{}, err error) {

	cypher := strings.Replace(SimpleConnectionsQuery, "{dataset}", params.Dataset, -1)
	cypher, err = subName(params.NeuronName, params.NeuronId, cypher)
	if err != nil {
		return
	}

	if params.FindInputs {
		cypher = strings.Replace(cypher, "{connection}", "<-[e:ConnectsTo]-", -1)
	} else {
		cypher = strings.Replace(cypher, "{connection}", "-[e:ConnectsTo]->", -1)
	}

	return store.makeRequest(cypher)
}

func subName(neuronName string, neuronId int64, cypher string) (string, error) {
	if neuronName != "" {
		cypher = strings.Replace(cypher, "{neuronid}", "name =~\""+neuronName+"\"", -1)
	} else if neuronId != 0 {
		cypher = strings.Replace(cypher, "{neuronid}", "bodyId = "+strconv.FormatInt(neuronId, 10), -1)
	} else {
		cypher = strings.Replace(cypher, "{neuronid}", "", -1)
		return cypher, fmt.Errorf("no neuron name specified")
	}

	return cypher, nil
}

// ExplorerROIsInNeuron implements API to show ROIs intersecting given neuron
func (store Store) ExplorerROIsInNeuron(params npexplorer.NeuronNameParams) (res interface{}, err error) {
	cypher := strings.Replace(IntersectingROIQuery, "{dataset}", params.Dataset, -1)
	cypher, err = subName(params.NeuronName, params.NeuronId, cypher)
	if err != nil {
		return
	}
	return store.makeRequest(cypher)
}

// ExplorerCommonConnectivity implements API to show common inputs or outputs to a set of neurons
func (store Store) ExplorerCommonConnectivity(params npexplorer.CommonConnectivityParams) (res interface{}, err error) {

	cypher := strings.Replace(CommonConnectivityQuery, "{dataset}", params.Dataset, -1)
	if params.FindInputs {
		cypher = strings.Replace(cypher, "{connection}", "<-[r:ConnectsTo]-", -1)
		cypher = strings.Replace(cypher, "{inputoroutput}", "input", -1)
	} else {
		cypher = strings.Replace(cypher, "{connection}", "-[r:ConnectsTo]->", -1)
		cypher = strings.Replace(cypher, "{inputoroutput}", "output", -1)
	}

	if params.AllSegments {
		cypher = strings.Replace(cypher, "{NeuronSegment}", "Segment", -1)
	} else {
		cypher = strings.Replace(cypher, "{NeuronSegment}", "Neuron", -1)
	}

	if params.NeuronIds != nil && len(params.NeuronIds) > 0 {
		cypher = strings.Replace(cypher, "{idorname}", "bodyId", -1)
		bodystr := ""
		for index, bodyid := range params.NeuronIds {
			if index != 0 {
				bodystr = bodystr + ","
			}
			bodystr = bodystr + strconv.FormatInt(bodyid, 10)
		}
		cypher = strings.Replace(cypher, "{neuron_list}", bodystr, -1)
	} else if params.NeuronNames != nil && len(params.NeuronNames) > 0 {
		cypher = strings.Replace(cypher, "{idorname}", "name", -1)
		bodystr := ""
		for index, bodyname := range params.NeuronNames {
			if index != 0 {
				bodystr = bodystr + ","
			}
			bodystr = bodystr + "\"" + bodyname + "\""
		}
		cypher = strings.Replace(cypher, "{neuron_list}", bodystr, -1)
	} else {
		return nil, fmt.Errorf("neuron ids or names not specified")
	}

	if params.PreThreshold > 0 {
		prestr := "(c.pre >= " + strconv.Itoa(params.PreThreshold) + ")"
		prestr = "AND " + prestr
		cypher = strings.Replace(cypher, "{pre_cond}", prestr, -1)
	} else {
		cypher = strings.Replace(cypher, "{pre_cond}", "", -1)
	}

	if params.PostThreshold > 0 {
		poststr := "(c.post >= " + strconv.Itoa(params.PostThreshold) + ")"
		poststr = "AND " + poststr
		cypher = strings.Replace(cypher, "{post_cond}", poststr, -1)
	} else {
		cypher = strings.Replace(cypher, "{post_cond}", "", -1)
	}

	statusarr := ""
	for index, status := range params.Statuses {
		if index == 0 {
			statusarr = statusarr + "AND "
			statusarr = statusarr + "("
		} else {
			statusarr = statusarr + " OR "
		}

		statusarr = statusarr + "c.status = \"" + status + "\""
	}
	if statusarr != "" {
		statusarr = statusarr + ")"
	}

	cypher = strings.Replace(cypher, "{status_conds}", statusarr, -1)

	return store.makeRequest(cypher)
}

// ExplorerAutapses implements API to find neurons with autapses for a dataset
func (store Store) ExplorerAutapses(params npexplorer.DatasetParams) (res interface{}, err error) {

	cypher := strings.Replace(AutapsesQuery, "{dataset}", params.Dataset, -1)
	return store.makeRequest(cypher)
}

// ExplorerDistribution implements API to find distribution segment sizes
func (store Store) ExplorerDistribution(params npexplorer.DistributionParams) (res interface{}, err error) {
	cypher := strings.Replace(DistributionQuery, "{dataset}", params.Dataset, -1)
	cypher = strings.Replace(cypher, "{ROI}", params.ROI, -1)
	if params.IsPre {
		cypher = strings.Replace(cypher, "{preorpost}", "pre", -1)
		cypher = strings.Replace(cypher, "{preorpost_filter}", "WHERE n.pre > 0", -1)
	} else {
		cypher = strings.Replace(cypher, "{preorpost}", "post", -1)
		cypher = strings.Replace(cypher, "{preorpost_filter}", "WHERE n.post > 0", -1)
	}

	return store.makeRequest(cypher)
}

// ExplorerCompleteness implements API to find percentage of volume covered by filtered neurons
func (store Store) ExplorerCompleteness(params npexplorer.CompletenessParams) (res interface{}, err error) {
	cypher := strings.Replace(CompletenessQuery, "{dataset}", params.Dataset, -1)
	if params.PreThreshold > 0 || params.PostThreshold > 0 || len(params.Statuses) > 0 {
		cypher = strings.Replace(cypher, "{has_conditions}", "WHERE", -1)
	} else {
		cypher = strings.Replace(cypher, "{has_conditions}", "", -1)
	}

	if params.AllSegments {
		cypher = strings.Replace(cypher, "{NeuronSegment}", "Segment", -1)
	} else {
		cypher = strings.Replace(cypher, "{NeuronSegment}", "Neuron", -1)
	}

	initcond := false
	if params.PreThreshold > 0 {
		prestr := "(n.pre >= " + strconv.Itoa(params.PreThreshold) + ")"
		cypher = strings.Replace(cypher, "{pre_cond}", prestr, -1)
		initcond = true
	} else {
		cypher = strings.Replace(cypher, "{pre_cond}", "", -1)
	}

	if params.PostThreshold > 0 {
		poststr := "(n.post >= " + strconv.Itoa(params.PostThreshold) + ")"
		if initcond {
			poststr = "AND " + poststr
		}
		initcond = true
		cypher = strings.Replace(cypher, "{post_cond}", poststr, -1)
	} else {
		cypher = strings.Replace(cypher, "{post_cond}", "", -1)
	}

	statusarr := ""
	for index, status := range params.Statuses {
		if index == 0 {
			if initcond {
				statusarr = statusarr + "AND "
			}
			statusarr = statusarr + "("
		} else {
			statusarr = statusarr + " OR "
		}

		statusarr = statusarr + "n.status = \"" + status + "\""
	}
	if statusarr != "" {
		statusarr = statusarr + ")"
	}

	cypher = strings.Replace(cypher, "{status_conds}", statusarr, -1)

	return store.makeRequest(cypher)
}
