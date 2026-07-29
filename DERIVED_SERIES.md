# Feature: derived series

I've commented the config.go to declare a new capability:
I'd like the benchviz to be able to construct derived aggregated series (e.g. min,max,mean,geomean) from other measurement series and add these new series to the configuration.

The idea is to present a summarized extra bar on the bar chart.
A few requirements:
* we may declare as "derived" either a Version or Category:
  * a derived Version creates a new version which applies the aggregation     function over all other (non derived) versions, keeping each function and aggregating over contexts. On the chart, this will show as a new X-axis point (or Y-axis for horizontal layout) corresponding to the derived version, with all functions side by side with their derived measurement.
  * a derived configuration creates a new configuration (new chart) with 
    a series aggregated over contexts _and_ versions
* a single context as "derived" is an invalid configuration
* multiple derived entries may be added (e.g. min _and_ max)- aggregation only applies to actual measurements, not to other derived configurations.
* derived series are computed once all other series have been formed, the specified order in the config determines the layout order
* the name of the new derived series can be specified. If not present, the 
  default is inferred from: [function name] - [AggregationFunction]


Do you need additional precisions or are we okay to start a development of this feature?

