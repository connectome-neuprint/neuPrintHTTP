package npexplorer

// ?! consider defining struct here and then storage can include this information

type DatasetParams struct {
	Dataset string `json:"dataset"`
}

type NeuronNameParams struct {
	DatasetParams
	NeuronName string `json:"neuron_name,omitempty"`
	NeuronId   int64  `json:"neuron_id,omitempty"`
}

type FilterParams struct {
	Statuses      []string `json:"statuses"`
	PreThreshold  int      `json:"pre_threshold"`
	PostThreshold int      `json:"post_threshold"`
	AllSegments   bool     `json:"all_segments,omitempty"`
}

type FindNeuronsParams struct {
	NeuronNameParams
	FilterParams
	InputROIs  []string `json:"input_ROIs"`
	OutputROIs []string `json:"output_ROIs"`
}

type ConnectionsParams struct {
	NeuronNameParams
	FindInputs bool `json:"find_inputs"`
}

type MetaValParams struct {
	DatasetParams
	KeyName string `json:"key_name"`
}

type CommonConnectivityParams struct {
	DatasetParams
	FilterParams
	FindInputs  bool     `json:"find_inputs"`
	NeuronIds   []int64  `json:"neuron_ids,omitempty"`
	NeuronNames []string `json:"neuron_names,omitempty"`
}

type DistributionParams struct {
	DatasetParams
	ROI   string `json:"ROI"`
	IsPre bool   `json:"is_pre"`
}

type CompletenessParams struct {
	DatasetParams
	FilterParams
}

// StorageAPI specifies the interface that backend engine needs to satisfy
type StorageAPI interface {
	ExplorerFindNeurons(FindNeuronsParams) (interface{}, error)
	ExplorerNeuronMeta(DatasetParams) (interface{}, error)
	ExplorerNeuronMetaVals(MetaValParams) (interface{}, error)
	ExplorerROIConnectivity(DatasetParams) (interface{}, error)
	ExplorerRankedTable(ConnectionsParams) (interface{}, error)
	ExplorerSimpleConnections(ConnectionsParams) (interface{}, error)
	ExplorerROIsInNeuron(NeuronNameParams) (interface{}, error)
	ExplorerCommonConnectivity(CommonConnectivityParams) (interface{}, error)
	ExplorerAutapses(DatasetParams) (interface{}, error)
	ExplorerDistribution(DistributionParams) (interface{}, error)
	ExplorerCompleteness(CompletenessParams) (interface{}, error)
}
