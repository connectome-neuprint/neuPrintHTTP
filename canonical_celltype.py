# need to install scipy, scikit-learn, pandas
import sys
import json
import re
import pandas as pd
import numpy as np
from scipy.spatial.distance import pdist
from scipy.spatial.distance import squareform
from sklearn.preprocessing import normalize
from sklearn.preprocessing import StandardScaler

# read json from piped string
input = json.loads(sys.stdin.read())



# maintain count of cell types
# make a list of cell type (or "none" if ID), count, partner id (to be used for none) for each cell type
# (ignore name exclusions and Leaves for this table unless that there is nothing than add both)
# (list for inputs and outputs)
unique_neurons = set(input["unique_neurons"])
good_neurons = set(input["good_neurons"])
celltype_lists_inputs_t = input["celltype_lists_inputs"]
celltype_lists_outputs_t = input["celltype_lists_outputs"]

input_size = input["input_size"]
input_size = {int(k):v for k, v in input_size.items()}

output_size = input["output_size"]
output_size = {int(k):v for k, v in output_size.items()}

input_comp = input["input_comp"]
input_comp = {int(k):v for k, v in input_comp.items()}

output_comp = input["output_comp"]
output_comp = {int(k):v for k, v in output_comp.items()}

neuron_instance = input["neuron_instance"]
neuron_instance = {int(k):v for k, v in neuron_instance.items()}

maxconn = 0
for neuron in good_neurons:
    numconn = 0
    if neuron in input_size:
        numconn += input_size[neuron]
    if neuron in output_size:
        numconn += output_size[neuron]
    if numconn > maxconn:
        maxconn = numconn

maxconn *= 0.5

good_neurons2 = []
for neuron in good_neurons:
    numconn = 0
    if neuron in input_size:
        numconn += input_size[neuron]
    if neuron in output_size:
        numconn += output_size[neuron]
    if numconn >= maxconn:
        good_neurons2.append(neuron)

good_neurons = good_neurons2

# unique neurons doesn't actually have to be a set, reorder
unique_neurons_new = []
for neuron in unique_neurons:
    if neuron in good_neurons:
        unique_neurons_new.append(neuron)
for neuron in unique_neurons:
    if neuron not in good_neurons:
        unique_neurons_new.append(neuron)
unique_neurons = unique_neurons_new

celltype_lists_inputs = {}
celltype_lists_outputs = {}

for bodyid, infoarr_arr in celltype_lists_inputs_t.items():
    bodyid = int(bodyid)
    if bodyid not in celltype_lists_inputs:
        celltype_lists_inputs[bodyid] = []

    # add body id in the middle to allow sorting
    for infoarr in infoarr_arr:
        celltype_lists_inputs[bodyid].append((infoarr[0], infoarr[1], {"partner": infoarr[2], "weight": infoarr[0], "hastype": infoarr[3], "important": False}))

for bodyid, infoarr_arr in celltype_lists_outputs_t.items():
    bodyid = int(bodyid)
    if bodyid not in celltype_lists_outputs:
        celltype_lists_outputs[bodyid] = []

    # add body id in the middle to allow sorting
    for infoarr in infoarr_arr:
        celltype_lists_outputs[bodyid].append((infoarr[0], infoarr[1], {"partner": infoarr[2], "weight": infoarr[0], "hastype": infoarr[3], "important": False}))


# ***** constants ****


importance_cutoff = 0.25 # ignore connections after the top 50% (with some error margin)
tracing_accuracy = 5 # number of connection error reasonably possible based on proofreading


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
        if neuron in celltype_lists:
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
            if neuron in celltype_lists:
                for (weight, ignore, val) in celltype_lists[neuron]:
                    if not val["examined"] and val["partner"] == entry["partner"] and val["hastype"] == entry["hastype"]:
                        val["examined"] = True
                        break

    # generate feature map for neuron
    features = np.zeros((len(unique_neurons), len(celltype_rank)))

    iter1 = 0
    for neuron in unique_neurons:
        connlist = []
        # it is possible that a neuron has no input or outputs
        if neuron in celltype_lists:
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
    dist_matrix = squareform(pdist(features.values))

    return pd.DataFrame(dist_matrix, index=features.index.values.tolist(), columns=features.index.values.tolist())

def normalize_data(inputs, outputs, neurons=None):
    if len(inputs.columns) == 0 and len(outputs.columns) == 0:
        return inputs, inputs.index.to_list()


    if neurons is not None:
        inputs = inputs.loc[neurons]
        outputs = outputs.loc[neurons]

    func = np.vectorize(lambda x: 1 / (1 + np.exp(-((x-17)/20))))

    if  len(inputs.columns) == 0:
        supercombo = normalize(func(outputs.values), axis=1, norm='l2')
    elif len(outputs.columns) == 0:
        supercombo = normalize(func(inputs.values), axis=1, norm='l2')
    else:
        input_norm = normalize(func(inputs.values), axis=1, norm='l2')
        output_norm = normalize(func(outputs.values), axis=1, norm='l2')
        supercombo = np.concatenate((input_norm*(0.5**(1/2)), output_norm*(0.5**(1/2))), axis=1)

    return supercombo, inputs.index.to_list()

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

# generate big input, output (using threshold) and show biggest additions and misses with 50% match threshold

# OUTPUT
celltypes_inputs = {}
celltypes_outputs = {}

celltypes_inputs_missed = {}
celltypes_outputs_missed = {}

### sort feature_inputs

fi_lim = features_inputs.loc[list(neuron_working_set)]
fo_lim = features_outputs.loc[list(neuron_working_set)]
imed = fi_lim.median()
omed = fo_lim.median()
i_order = np.argsort(imed)[::-1]
o_order = np.argsort(omed)[::-1]

features_inputs = features_inputs.iloc[:, i_order]
features_outputs = features_outputs.iloc[:, o_order]
#####


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
        newdict[key] = df.to_dict('split')
    return newdict

results["neuroninfo"] = neuroninfo
results["centroid-neuron"] = centroid_neuron
results["neuron-inputs"] = dictdf_to_json(celltypes_inputs)
results["neuron-outputs"] = dictdf_to_json(celltypes_outputs)

if len(neuroninfo) > 1:
    results["dist-matrix"] = dist_matrix.to_dict('split')
    results["average-distance"] = dist_matrix.values.sum()/(len(all_features)*len(all_features)-len(all_features))
    results["neuron-missed-inputs"] = dictdf_to_json(celltypes_inputs_missed)
    results["neuron-missed-outputs"] = dictdf_to_json(celltypes_outputs_missed)
    results["common-inputs"] = features_inputs.to_dict('split')
    results["common-outputs"] = features_outputs.to_dict('split')
    medin_df = pd.DataFrame({'median': feature_inputs_med})
    medout_df = pd.DataFrame({'median': feature_outputs_med})
    results["common-inputs-med"] = medin_df.to_dict('split')
    results["common-outputs-med"] = medout_df.to_dict('split')


print(json.dumps(results, indent=2))

