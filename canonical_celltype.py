# need to install neuprint-python, umap, scipy, scikit-learn, pandas
# TODO: modify neuprint python to pass dataset into custom

import sys
import json
import os

dataset = sys.argv[1]
typename = sys.argv[2]
server = os.environ["NEUPRINT_SERVER"]

#server = sys.argv[3]
#token = sys.argv[4]


import neuprint as neu
client = neu.Client(server)

# fetch all connections from this cell type
query = f"MATCH (n :`{dataset}_Neuron` {{type: \"{typename}\"}})-[x :ConnectsTo]-(m) RETURN n.bodyId AS bodyId, n.instance AS instance, x.weight AS weight, m.bodyId AS bodyId2, m.type AS type2, (startNode(x) = n) as isOutput, n.status AS body1status, m.status AS body2status"
connections = neu.fetch_custom(query)

import re
import pandas as pd
import numpy as np

# ***** constants ****
primary_status = set(["Traced", "Roughly traced", "Leaves"]) # won't consider a type unless at least Leaves
connection_status = set(["Traced", "Roughly traced"]) # no need to look at partners to leaves
name_exclusions = ".*_L" # ignore anything on the left unless there is nothing on the righ
importance_cutoff = 0.25 # ignore connections after the top 50% (with some error margin)
minweight = 3 # ignore connections for canonical inputs or outputs that are below this
tracing_accuracy = 5 # number of connection error reasonably possible based on proofreading
degrade_threshold = 7 # connection strength below which one does not weight as heavily

# maintain count of cell types
unique_neurons = set()
good_neurons = set()

# make a list of cell type (or "none" if ID), count, partner id (to be used for none) for each cell type
# (ignore name exclusions and Leaves for this table unless that there is nothing than add both)
# (list for inputs and outputs) 
celltype_lists_inputs = {}
celltype_lists_outputs = {}
input_size = {}
output_size = {}
input_comp = {}
output_comp = {}
neuron_instance = {}
if len(connections) == 0:
    print(json.dumps({}))
    exit(0)
for idx, row in connections.iterrows():
    bodyid = row["bodyId"]
    type_status = row["body1status"]
    type_status2 = row["body2status"] 
    is_output = row["isOutput"]
    neuron_instance[bodyid] = row["instance"]
    
    # do not consider untraced neurons
    if type_status not in primary_status:
        continue
    unique_neurons.add(bodyid)
        
    # add stats
    if is_output:
        if bodyid not in output_size:
            output_size[bodyid] = 0
        output_size[bodyid] += row["weight"]
    if not is_output:
        if bodyid not in input_size:
            input_size[bodyid] = 0
        input_size[bodyid] += row["weight"]
        
    # might as well ignore connection as well if not to traced
    if type_status2 not in primary_status:
        continue

    # add stats if traced
    if is_output:
        if bodyid not in output_comp:
            output_comp[bodyid] = 0
        output_comp[bodyid] += row["weight"]
    if not is_output:
        if bodyid not in input_comp:
            input_comp[bodyid] = 0
        input_comp[bodyid] += row["weight"]
        
    conntype = row["type2"]
    hastype = True
    if conntype is None or conntype == "":
        conntype = str(row["bodyId2"])
        hastype = False

    # don't consider the edge for something that is leaves and has not type
    if not hastype and type_status2 not in connection_status:
        continue
        
    # don't consider a weak edge
    if row["weight"] < minweight:
        continue
        
    if type_status in connection_status:
        # make sure name exclusions are not in the instance name
        if re.search(name_exclusions, row["instance"]) is None:
            good_neurons.add(bodyid)
    
    if is_output:       
        if bodyid not in celltype_lists_outputs:
            celltype_lists_outputs[bodyid] = []

        # add body id in the middle to allow sorting
        celltype_lists_outputs[bodyid].append((row["weight"], row["bodyId2"], {"partner": conntype, "weight": row["weight"], "hastype": hastype, "important": False}))
    else:       
        if bodyid not in celltype_lists_inputs:
            celltype_lists_inputs[bodyid] = []

        # add body id in the middle to allow sorting
        celltype_lists_inputs[bodyid].append((row["weight"], row["bodyId2"], {"partner": conntype, "weight": row["weight"], "hastype": hastype, "important": False}))

# OUTPUT
neuroninfo = {}
for neuron in unique_neurons:
    info = {}
    info["input-size"] = 0
    info["output-size"] = 0
    info["input-comp"] = 0
    info["input-comp"] = 0
    if neuron in input_size:
        info["input-size"] = input_size[neuron]
    if neuron in input_comp:
        info["input-comp"] = input_comp[neuron]
    if neuron in output_size:
        info["output-size"] = output_size[neuron]
    if neuron in output_comp:
        info["output-comp"] = output_comp[neuron]
    if neuron in good_neurons:
        info["reference"] = True
    else:
        info["reference"] = False
    info["instance-name"] = neuron_instance[neuron]
    neuroninfo[neuron] = info

# sort each list and cut-off anything below 50% with some error bar  
def sort_lists(cell_type_lists):  
    for bid, info in cell_type_lists.items():
        important_count = 0
        info.sort()
        info.reverse()
        total = 0
        for (weight, ignore, val) in info:
            total += weight

        count = 0
        threshold = 0
        for (weight, ignore, val) in info:
            # ignore connections below thresholds (but take at least 5 inputs and outputs)
            if weight < threshold and important_count > 5:
                break
            important_count += 1
            val["important"] = True

            count += weight
            # set threshold based on connection that gets to 50%
            if threshold == 0 and count > (total*importance_cutoff):
                threshold = weight - weight**(1/2)
sort_lists(celltype_lists_inputs)
sort_lists(celltype_lists_outputs)

# if the list of good neurons is empty, use these neurons to choose features but analyze all
neuron_working_set = good_neurons
if len(good_neurons) == 0:
    neuron_working_set = unique_neurons
    
def generate_feature_table(celltype_lists):
    # add shared dict to a globally sorted list from each partner
    global_queue = []
    for neuron in neuron_working_set:
        for (weight, bid2, val) in celltype_lists[neuron]:
            val["examined"] = False
            if val["important"]:
                global_queue.append((weight, bid2, neuron, val))
    global_queue.sort()
    global_queue.reverse()
    
    # provide rank from big to small and provide connection count
    # (count delineated by max in row but useful for bookkeeping)
    celltype_rank = []
    
    # from large to small make a list of common partners, each time another is added, iterate through
    # other common partners to find a match (though we need to look at the whole list beyond the cut-off)
    for (count, ignore, neuron, entry) in global_queue:
        if entry["examined"]:
            continue
        celltype_rank.append((entry["partner"], count, entry["hastype"]))
    
        # check for matches for each cell type instance in neuron working set
        for neuron in neuron_working_set:
            for (weight, ignore, val) in celltype_lists[neuron]:
                if not val["examined"] and val["partner"] == entry["partner"] and val["hastype"] == entry["hastype"]:
                    val["examined"] = True
                    break
                    
    # generate feature map for neuron
    features = np.zeros((len(unique_neurons), len(celltype_rank)))
    
    iter1 = 0
    for neuron in unique_neurons:
        connlist = celltype_lists[neuron]
        match_list = {}
        for (weight, ignore, val) in connlist:
            matchkey = (val["partner"], val["hastype"])
            if matchkey not in match_list:
                match_list[matchkey] = []
            match_list[matchkey].append(weight)
        cfeatures = []
        for (ctype, count, hastype) in celltype_rank:
            rank_match = (ctype, hastype)
            if rank_match in match_list:
                cfeatures.append(match_list[rank_match][0])
                del match_list[rank_match][0]
                if len(match_list[rank_match]) == 0:
                    del match_list[rank_match]
            else:
                cfeatures.append(0)
        features[iter1] = cfeatures
        iter1 +=1
            
    ranked_names = []
    for (ctype, count, hastype) in celltype_rank:
        ranked_names.append(ctype)
    neuron_ids = []
    for neuron in unique_neurons:
        neuron_ids.append(neuron)
        
        
    return pd.DataFrame(features, index=neuron_ids, columns=ranked_names)
    
# OUTPUT 
features_inputs = generate_feature_table(celltype_lists_inputs)

# OUTPUT
features_outputs = generate_feature_table(celltype_lists_outputs)


def compute_distance_matrix(features):
    """Compute a distance matrix between the neurons.

    Args:
        features (dataframe): matrix of features
    Returns:
        (dataframe): A 2d distance matrix represented as a table                                                      
    """

    # compute pairwise distance and put in square form                                                                
    from scipy.spatial.distance import pdist
    from scipy.spatial.distance import squareform                                                                     
    dist_matrix = squareform(pdist(features.values))                                                                  

    return pd.DataFrame(dist_matrix, index=features.index.values.tolist(), columns=features.index.values.tolist())   

def normalize_data(inputs, outputs, neurons=None):
    if neurons is not None:
        inputs = inputs.loc[neurons]
        outputs = outputs.loc[neurons]
    
    # normalize similar to cblast (but do not scale features after scaling across a neuron)
    from sklearn.preprocessing import normalize
    from sklearn.preprocessing import StandardScaler
    #combofeatures = np.concatenate((inputs.values, outputs.values), axis=1)
    #scaledfeatures = StandardScaler().fit_transform(combofeatures)
    #scaledfeatures = np.log(combofeatures+1)
    #scaledfeatures_norm = normalize(scaledfeatures, axis=1, norm='l2')
    #combofeatures_norm = normalize(combofeatures, axis=1, norm='l2') 
    # ?? add size
    #supercombo = np.concatenate((combofeatures_norm*(0.4**(1/2)), scaledfeatures_norm*(0.6**(1/2))), axis=1)
    
    # should input and output be weighted by relative size??
    
    #func = np.vectorize(lambda x: 0 if x == 0 else (1 / (1 + np.exp(-((x-8)/2))) if x<=7 else 1 / (1 + np.exp(-((x-17)/20)))))
    #func = np.vectorize(lambda x: 0 if x == 0 else 1 / (1 + np.exp(-((x-17)/20))))
    func = np.vectorize(lambda x: 1 / (1 + np.exp(-((x-17)/20))))
    #func = np.vectorize(lambda x: x)
    input_norm = normalize(func(inputs.values), axis=1, norm='l2')
    output_norm = normalize(func(outputs.values), axis=1, norm='l2')
    
    #input_norm = normalize(np.log(inputs.values+1), axis=1, norm='l2')
    #output_norm = normalize(np.log(outputs.values+1), axis=1, norm='l2')
    #input_norm = normalize(inputs.values, axis=1, norm='l2')
    #output_norm = normalize(outputs.values, axis=1, norm='l2')
    supercombo = np.concatenate((input_norm*(0.5**(1/2)), output_norm*(0.5**(1/2))), axis=1)
    
    return supercombo, inputs.index.to_list()
    #return combofeatures_norm, inputs.index.to_list()

all_features, row_ids_all = normalize_data(features_inputs, features_outputs)
working_features, row_ids = normalize_data(features_inputs, features_outputs, list(neuron_working_set))
# ?? add size features, better normalization?

# find representativee sample from neuron_working_set subset
avg_features = np.mean(working_features, axis=0)
minval = 99999999999999

# OUTPUT
centroid_neuron = -1 # default neuron to use ("the canonical neuron")
for iter1 in range(len(working_features)):
    tval = ((avg_features-working_features[iter1])**2).sum()
    if tval < minval:
        minval = tval
        centroid_neuron = row_ids[iter1]

# compute distance matrix of features
# OUTPUT
dist_matrix = compute_distance_matrix(pd.DataFrame(all_features, index=row_ids_all))


# reduce features using umap (require at least 4 neurons)
# OUTPUT
feature_matrix = None
if len(all_features) >= 4:
    import umap
    reducer = umap.UMAP()
    umap_vals = reducer.fit_transform(all_features)
    feature_matrix = pd.DataFrame(umap_vals, index=row_ids_all) # can visualize using typecluster.view

# generate big input, output (using threshold) and show biggest additions and misses with 50% match threshold    

# OUTPUT
celltypes_inputs = {}
celltypes_outputs = {}

celltypes_inputs_missed = {}
celltypes_outputs_missed = {}

# get median value for each feature
feature_inputs_lim = features_inputs.loc[list(neuron_working_set)]
feature_outputs_lim = features_outputs.loc[list(neuron_working_set)]

# OUTPUT
feature_inputs_med = feature_inputs_lim.median()   
feature_outputs_med = feature_outputs_lim.median()


#  get matches for inputs or outputs
def get_matches(celltype_lists, feature_med, celltypes_io, celltypes_io_missed):
    # cutoff for match
    match_cutoff = 0.5

    for bid, connarr in celltype_lists.items():
        res = []

        matched_ids = set()
        neuron2rank = {}
        rank2neuron = {}
        for idx in range(len(connarr)):
            (weight, ignore, info) = connarr[idx]
            
            match_features = list(np.where(feature_med.index.values == info["partner"])[0])
            #match_features = feature_med[[info["partner"]]]
            matchid = -1
            matchval = 0
            for fid in match_features:
                if fid not in matched_ids:
                    # only consider a match if within a certain amount
                    if matchid == -1:
                        matchid = fid
                        matchval = feature_med.iloc[fid]
                    diff = abs(feature_med.iloc[fid] - weight)
                    if feature_med.iloc[fid] > weight*match_cutoff and feature_med.iloc[fid]*match_cutoff < weight or diff <= tracing_accuracy:
                        matchid = fid
                        matchval = feature_med.iloc[fid]
                        break
           
            # add a non-matching id (-1 by default)
            matched_ids.add(matchid)
            neuron2rank[idx] = matchval
            rank2neuron[matchid] = matchval

        # load top io and match weight
        for idx in range(len(connarr)):
            (weight, ignore, info) = connarr[idx]
            # array is sorted
            if not info["important"]:
                break
            goodmatch = False
            diff = abs(neuron2rank[idx] - weight)
            if (neuron2rank[idx] > (weight*match_cutoff)) and ((neuron2rank[idx]*match_cutoff) < weight) or (diff <= tracing_accuracy):
                goodmatch = True

            res.append([info["partner"], weight, neuron2rank[idx], goodmatch])
        celltypes_io[bid] = pd.DataFrame(res, columns=["type", "neuron weight", "group weight", "good match"])

        # load top missing io (only show those outside 0.5 threshold or 5 connections away)
        res2 = []
        for fid in range(len(feature_med)):
            weight = 0
            if fid in rank2neuron:
                weight = rank2neuron[fid]
            #if feature_med[fid] <= weight*match_cutoff or feature_med[fid]*match_cutoff >= weight:
            if feature_med[fid]*match_cutoff >= weight and feature_med[fid] > (weight+tracing_accuracy):
                res2.append([feature_med.index[fid], feature_med[fid], weight])
        celltypes_io_missed[bid] = pd.DataFrame(res2, columns=["type", "group weight", "neuron weight"])
    
get_matches(celltype_lists_inputs, feature_inputs_med, celltypes_inputs, celltypes_inputs_missed)
get_matches(celltype_lists_outputs, feature_outputs_med, celltypes_outputs, celltypes_outputs_missed)


results = {}

def dictdf_to_json(val):
    if val is None:
        return None
    newdict = {}
    for key, df in val.items():
        newdict[key] = df.to_json(orient='split')
    return newdict

results["neuroninfo"] = neuroninfo
results["centroid-neuron"] = centroid_neuron
results["neuron-inputs"] = dictdf_to_json(celltypes_inputs)
results["neuron-outputs"] = dictdf_to_json(celltypes_outputs)
results["neuron-missed-inputs"] = dictdf_to_json(celltypes_inputs_missed)
results["neuron-missed-outputs"] = dictdf_to_json(celltypes_outputs_missed)
results["common-inputs"] = features_inputs.to_json(orient='split')
results["common-outputs"] = features_outputs.to_json(orient='split')
if feature_matrix is None:
    results["scatter2D-cluster"] = None 
else:
    results["scatter2D-cluster"] = feature_matrix.to_json(orient='split')
results["dist-matrix"] = dist_matrix.to_json(orient='split')
results["average-distance"] = dist_matrix.values.sum()/(len(all_features)*len(all_features)-len(all_features))
results["common-inputs-med"] = feature_inputs_med.to_json(orient='split')
results["common-outputs-med"] = feature_outputs_med.to_json(orient='split')


print(json.dumps(results, indent=2))

