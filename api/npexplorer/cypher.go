package npexplorer

import (
	"fmt"
	"strconv"
	"strings"
)

const (
	NeuronMetaQuery = "MATCH (n :Neuron) UNWIND KEYS(n) AS x RETURN DISTINCT x AS pname"

	NeuronMetaValsQuery = "MATCH (n :Neuron) RETURN DISTINCT n.{metakey} AS val"

	ROIQuery = "MATCH (neuron :Neuron) RETURN neuron.bodyId AS bodyid, neuron.roiInfo AS roiInfo"

	AutapsesQuery = "MATCH (n:Neuron)-[x:ConnectsTo]->(n) RETURN n.bodyId AS id, x.weight AS weight, n.instance AS name, n.type AS type ORDER BY x.weight DESC"

	CompletenessQuery = "MATCH (n:{NeuronSegment}) {has_conditions} {pre_cond} {post_cond} {status_conds} WITH apoc.convert.fromJsonMap(n.roiInfo) AS roiInfo WITH roiInfo AS roiInfo, keys(roiInfo) AS roiList UNWIND roiList AS roiName WITH roiName AS roiName, sum(roiInfo[roiName].pre) AS pre, sum(roiInfo[roiName].post) AS post MATCH (meta:Meta) WITH apoc.convert.fromJsonMap(meta.roiInfo) AS globInfo, roiName AS roiName, pre AS pre, post AS post RETURN roiName AS unlabelres, pre AS roipre, post AS roipost, globInfo[roiName].pre AS totalpre, globInfo[roiName].post AS totalpost ORDER BY roiName"

	DistributionQuery = "MATCH (n:Segment {`{ROI}`: true}) {preorpost_filter} WITH n.bodyId as bodyId, apoc.convert.fromJsonMap(n.roiInfo)[\"{ROI}\"].{preorpost} AS {preorpost}size WHERE {preorpost}size > 0 WITH collect({id: bodyId, {preorpost}: {preorpost}size}) as bodyinfoarr, sum({preorpost}size) AS tot UNWIND bodyinfoarr AS bodyinfo RETURN bodyinfo.id AS id, bodyinfo.{preorpost} AS size, tot AS total ORDER BY bodyinfo.{preorpost} DESC"

	IntersectingROIQuery = "MATCH (neuron :Neuron) WHERE {neuronid} RETURN neuron.bodyId AS bodyid, neuron.instance AS bodyname, neuron.type AS bodytype, neuron.roiInfo AS roiInfo ORDER BY neuron.bodyId"

  SimpleConnectionsQuery = " MATCH (m:Meta) WITH m.superLevelRois AS rois MATCH (m:Neuron){connection}(n:Segment) WHERE {neuronid} RETURN m.instance AS Neuron1, m.type AS Neuron1Type, n.instance AS Neuron2, n.type AS Neuron2Type, n.bodyId AS Neuron2Id, e.weight AS Weight, m.bodyId AS Neuron1Id, n.status AS Neuron2Status, n.roiInfo AS Neuron2RoiInfo, n.size AS Neuron2Size, n.pre AS Neuron2Pre, n.post AS Neuron2Post, rois, e.weightHP AS WeightHP ORDER BY m.type, m.bodyId, e.weight DESC"

	RankedTableQuery = "MATCH (m:Neuron)-[e:ConnectsTo]-(n) WHERE {neuronid} RETURN m.instance AS Neuron1, m.type AS Neuron1Type, n.instance AS Neuron2, n.type AS Neuron2Type, e.weight AS Weight, n.bodyId AS Body2, id(m) AS m_id, id(n) AS n_id, id(startNode(e)) AS pre_id, m.bodyId AS Body1, e.weightHP AS WeightHP ORDER BY m.bodyId, e.weight DESC"

	FindNeuronsQuery = " MATCH (m:Meta) WITH m.superLevelRois AS rois MATCH (neuron :{NeuronSegment}) {has_conditions} {neuronid} {pre_cond} {post_cond} {status_conds} {roi_list} RETURN neuron.bodyId AS bodyid, neuron.instance AS bodyname, neuron.type AS bodytype, neuron.status AS neuronStatus, neuron.roiInfo AS roiInfo, neuron.size AS size, neuron.pre AS npre, neuron.post AS npost, rois, neuron.notes as notes ORDER BY neuron.bodyId"

	CommonConnectivityQuery = "WITH [{neuron_list}] AS queriedNeurons MATCH (k:{NeuronSegment}){connection}(c) WHERE (k.{idortype} IN queriedNeurons {pre_cond} {post_cond} {status_conds}) WITH k, c, r, toString(k.{idortype})+\"_weight\" AS dynamicWeight RETURN collect(apoc.map.fromValues([\"{inputoroutput}\", c.bodyId, \"name\", c.instance, \"type\", c.type, dynamicWeight, r.weight])) AS map"
)

// ExplorerFindNeurons implements API to find neurons in a certain ROI
func (store cypherAPI) ExplorerFindNeurons(params FindNeuronsParams) (res interface{}, err error) {
	cypher := FindNeuronsQuery

	initcond := false
	cypher, err2 := subName(params.NeuronName, params.NeuronId, "neuron", cypher, !params.EnableContains)
	// if name exists, then add where statement
	if err2 == nil {
		initcond = true
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

	return store.Store.GetMain(params.Dataset).CypherRequest(cypher, true)
}

// ExplorerNeuronMetaVals implements API to find distinct values for a given meta key stored for the dataset
func (store cypherAPI) ExplorerNeuronMetaVals(params MetaValParams) (res interface{}, err error) {
	cypher := NeuronMetaValsQuery
	cypher = strings.Replace(cypher, "{metakey}", params.KeyName, -1)
	return store.Store.GetMain(params.Dataset).CypherRequest(cypher, true)
}

// ExplorerNeuronMeta implements API to find meta information stored for the dataset
func (store cypherAPI) ExplorerNeuronMeta(params DatasetParams) (res interface{}, err error) {
	cypher := NeuronMetaQuery
	return store.Store.GetMain(params.Dataset).CypherRequest(cypher, true)
}

// ExplorerROIConnectivity implements API to find how ROIs are connected
func (store cypherAPI) ExplorerROIConnectivity(params DatasetParams) (res interface{}, err error) {
	cypher := ROIQuery
	return store.Store.GetMain(params.Dataset).CypherRequest(cypher, true)
}

// ExplorerRankedTable implements API to show connectivity broken down by cell type
func (store cypherAPI) ExplorerRankedTable(params ConnectionsParams) (res interface{}, err error) {
	cypher := RankedTableQuery
	cypher, err = subName(params.NeuronName, params.NeuronId, "m", cypher, !params.EnableContains)
	if err != nil {
		return
	}
	return store.Store.GetMain(params.Dataset).CypherRequest(cypher, true)
}

// ExplorerSimpleConnections implements API to show connectivity for a give neuron
func (store cypherAPI) ExplorerSimpleConnections(params ConnectionsParams) (res interface{}, err error) {
	cypher := SimpleConnectionsQuery
	cypher, err = subName(params.NeuronName, params.NeuronId, "m", cypher, !params.EnableContains)
	if err != nil {
		return
	}

	if params.FindInputs {
		cypher = strings.Replace(cypher, "{connection}", "<-[e:ConnectsTo]-", -1)
	} else {
		cypher = strings.Replace(cypher, "{connection}", "-[e:ConnectsTo]->", -1)
	}

	return store.Store.GetMain(params.Dataset).CypherRequest(cypher, true)
}

func subName(neuronName string, neuronId int64, matchvar string, cypher string, regex bool) (string, error) {
	regstr := "=~"
	if !regex {
		regstr = " CONTAINS "
	}
	if neuronName != "" {
		cypher = strings.Replace(cypher, "{neuronid}", "("+matchvar+".type"+regstr+"\""+neuronName+"\" OR "+matchvar+".instance"+regstr+"\""+neuronName+"\")", -1)
	} else if neuronId != 0 {
		cypher = strings.Replace(cypher, "{neuronid}", matchvar+".bodyId = "+strconv.FormatInt(neuronId, 10), -1)
	} else {
		cypher = strings.Replace(cypher, "{neuronid}", "", -1)
		return cypher, fmt.Errorf("no neuron name specified")
	}

	return cypher, nil
}

// ExplorerROIsInNeuron implements API to show ROIs intersecting given neuron
func (store cypherAPI) ExplorerROIsInNeuron(params NeuronNameParams) (res interface{}, err error) {
	cypher := IntersectingROIQuery
	cypher, err = subName(params.NeuronName, params.NeuronId, "neuron", cypher, true)
	if err != nil {
		return
	}
	return store.Store.GetMain(params.Dataset).CypherRequest(cypher, true)
}

// ExplorerCommonConnectivity implements API to show common inputs or outputs to a set of neurons
func (store cypherAPI) ExplorerCommonConnectivity(params CommonConnectivityParams) (res interface{}, err error) {
	cypher := CommonConnectivityQuery
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
		cypher = strings.Replace(cypher, "{idortype}", "bodyId", -1)
		bodystr := ""
		for index, bodyid := range params.NeuronIds {
			if index != 0 {
				bodystr = bodystr + ","
			}
			bodystr = bodystr + strconv.FormatInt(bodyid, 10)
		}
		cypher = strings.Replace(cypher, "{neuron_list}", bodystr, -1)
	} else if params.NeuronNames != nil && len(params.NeuronNames) > 0 {
		cypher = strings.Replace(cypher, "{idortype}", "type", -1)
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

	return store.Store.GetMain(params.Dataset).CypherRequest(cypher, true)
}

// ExplorerAutapses implements API to find neurons with autapses for a dataset
func (store cypherAPI) ExplorerAutapses(params DatasetParams) (res interface{}, err error) {
	cypher := AutapsesQuery
	return store.Store.GetMain(params.Dataset).CypherRequest(cypher, true)
}

// ExplorerDistribution implements API to find distribution segment sizes
func (store cypherAPI) ExplorerDistribution(params DistributionParams) (res interface{}, err error) {
	cypher := DistributionQuery
	cypher = strings.Replace(cypher, "{ROI}", params.ROI, -1)
	if params.IsPre {
		cypher = strings.Replace(cypher, "{preorpost}", "pre", -1)
		cypher = strings.Replace(cypher, "{preorpost_filter}", "WHERE n.pre > 0", -1)
	} else {
		cypher = strings.Replace(cypher, "{preorpost}", "post", -1)
		cypher = strings.Replace(cypher, "{preorpost_filter}", "WHERE n.post > 0", -1)
	}

	return store.Store.GetMain(params.Dataset).CypherRequest(cypher, true)
}

// ExplorerCompleteness implements API to find percentage of volume covered by filtered neurons
func (store cypherAPI) ExplorerCompleteness(params CompletenessParams) (res interface{}, err error) {
	cypher := CompletenessQuery
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

	return store.Store.GetMain(params.Dataset).CypherRequest(cypher, true)
}
