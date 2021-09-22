#decisiontreeplay

it is a place to test the decision tree.
when you run it, you will see the representation of the tree,
the input and the result after pass the payload to the resolver.

# The library
The library used for this project is https://github.com/tkanos/go-dtree so all
the documentation there is valid here. The version is v0.5.0 

# How does it works
+ Define a tree
+ Get the attributes of the payload
+ Pass the attributes to the tree resolver
+ get the name of the result node.

## Define a tree
You need a json file which will be a list of nodes, and except root, they have a parent id which indicates how are
connected. There are nodes that have `id` and `name` like root, "metric" and "event" in the next example

```json
[
  {
    "id": 1,
    "name": "root"
  },
  {
    "id": 2,
    "parent_id": 1,
    "key": "firstMetricIs",
    "operator": "eq",
    "value": "Device Control/Scan Rate ms"
  },
  {
    "id": 3,
    "parent_id": 1,
    "key": "firstMetricIs",
    "operator": "ne",
    "value": "Device Control/Scan Rate ms"
  },
  {
    "id": 4,
    "parent_id": 2,
    "Name": "metric"
  },
  {
    "id": 5,
    "parent_id": 3,
    "Name": "event"
  }
]
```

Representation
```
                                            ╭────╮                                             
                                            │root│                                             
                                            ╰──┬─╯                                             
                       ╭───────────────────────┴───────────────────────╮                       
╭──────────────────────┴─────────────────────╮  ╭──────────────────────┴─────────────────────╮ 
│firstMetricIs eq Device Control/Scan Rate ms│  │firstMetricIs ne Device Control/Scan Rate ms│ 
╰──────────────────────┬─────────────────────╯  ╰──────────────────────┬─────────────────────╯ 
                       │                                               │                       
                   ╭───┴──╮                                         ╭──┴──╮                    
                   │metric│                                         │event│                    
                   ╰──────╯                                         ╰─────╯         
```
And there are others that have `key`, `operator` and `value`. They could have a name which it don't impact in the 
tree's resolution but in the string representation is mask the key operator value label in the representation which is
only used to visualize the tree like in this example:
```json
[
  {
    "id": 1,
    "name": "root"
  },
  {
    "id": 2,
    "parent_id": 1,
    "key": "firstMetricIs",
    "operator": "eq",
    "value": "Device Control/Scan Rate ms",
    "name": "payload first metric is Scan rate"
  },
  {
    "id": 3,
    "parent_id": 1,
    "key": "firstMetricIs",
    "operator": "ne",
    "value": "Device Control/Scan Rate ms",
    "name": "payload first metric is not Scan rate"
  },
  {
    "id": 4,
    "parent_id": 2,
    "Name": "metric"
  },
  {
    "id": 5,
    "parent_id": 3,
    "Name": "event"
  }
]
```
Representation:
```
                                  ╭────╮                                   
                                  │root│                                   
                                  ╰──┬─╯                                   
                 ╭───────────────────┴─────────────────╮                   
╭────────────────┴────────────────╮ ╭──────────────────┴──────────────────╮
│payload first metric is Scan rate│ │payload first metric is not Scan rate│
╰────────────────┬────────────────╯ ╰──────────────────┬──────────────────╯
                 │                                     │                   
             ╭───┴──╮                               ╭──┴──╮                
             │metric│                               │event│                
             ╰──────╯                               ╰─────╯  
```
Also, you can use a fallback as value, so in case the conditions is not meet it can go the fallback like:

```json
[
  {
    "id": 1,
    "name": "root"
  },
  {
    "id": 2,
    "parent_id": 1,
    "key": "firstMetricIs",
    "operator": "eq",
    "value": "Device Control/Scan Rate ms"
  },
  {
    "id": 3,
    "parent_id": 1,
    "value": "fallback"
  },
  {
    "id": 4,
    "parent_id": 2,
    "Name": "metric"
  },
  {
    "id": 5,
    "parent_id": 3,
    "Name": "event"
  }
]
```
Representation:
```
                          ╭────╮                           
                          │root│                           
                          ╰──┬─╯                           
                       ╭─────┴───────────────────────╮     
╭──────────────────────┴─────────────────────╮  ╭────┴───╮ 
│firstMetricIs eq Device Control/Scan Rate ms│  │fallback│ 
╰──────────────────────┬─────────────────────╯  ╰────┬───╯ 
                       │                             │     
                   ╭───┴──╮                       ╭──┴──╮  
                   │metric│                       │event│  
                   ╰──────╯                       ╰─────╯ 
```

## Get the attributes of the payload
The operation nodes work with a key, operator and a value. The operator and the value are only especify in the tree.
So to know the list of operations and how the works click [here](https://github.com/tkanos/go-dtree#available-operators-)

The key is a different because not any key works. In this project there is package called `message` which is the one
that define what keys can be used. So in the function Attributes it receives the topic and payload so to add a new attribute
you will need to implement it there.

```go
func Attributes(topic string, payload pb.Payload) map[string]interface{} {
	attr := make(map[string]interface{})
	attr["firstMetricIs"] = firstMetric(payload)
	return attr
}
```
As an example lets implement number of metrics like this:
```go
func Attributes(topic string, payload pb.Payload) map[string]interface{} {
	attr := make(map[string]interface{})
	attr["firstMetricIs"] = firstMetric(payload)
	attr["metricsLen"] = fmt.Sprintf("%d", len(payload.Metrics))
	return attr
}
```
So in this case now I can use the key metricsLen like this:
```json
[
  {
    "id": 1,
    "name": "root"
  },
  {
    "id": 2,
    "parent_id": 1,
    "key": "firstMetricIs",
    "operator": "ne",
    "value": "Device Control/Scan Rate ms"
  },
  {
    "id": 3,
    "parent_id": 2,
    "key": "metricsLen",
    "operator": "eq",
    "value": "1"
  },
  {
    "id": 4,
    "parent_id": 3,
    "Name": "event"
  },
  {
    "id": 5,
    "parent_id": 1,
    "value": "fallback"
  },
  {
    "id": 6,
    "parent_id": 5,
    "name": "metric"
  },
  {
    "id": 7,
    "parent_id": 2,
    "value": "fallback"
  },
  {
    "id": 8,
    "parent_id": 7,
    "name": "metric"
  }
]
```

representation:
```
                          ╭────╮                           
                          │root│                           
                          ╰──┬─╯                           
                       ╭─────┴───────────────────────╮     
╭──────────────────────┴─────────────────────╮  ╭────┴───╮ 
│firstMetricIs ne Device Control/Scan Rate ms│  │fallback│ 
╰──────────────────────┬─────────────────────╯  ╰────┬───╯ 
                ╭──────┴───────╮                     │     
        ╭───────┴───────╮ ╭────┴───╮             ╭───┴──╮  
        │metricsLen eq 1│ │fallback│             │metric│  
        ╰───────┬───────╯ ╰────┬───╯             ╰──────╯  
                │              │                           
             ╭──┴──╮       ╭───┴──╮                        
             │event│       │metric│                        
             ╰─────╯       ╰──────╯    
```

## Get the name of the result node
decisiontreeplay output look like this:
```
tree:
                          ╭────╮                           
                          │root│                           
                          ╰──┬─╯                           
                       ╭─────┴───────────────────────╮     
╭──────────────────────┴─────────────────────╮  ╭────┴───╮ 
│firstMetricIs ne Device Control/Scan Rate ms│  │fallback│ 
╰──────────────────────┬─────────────────────╯  ╰────┬───╯ 
                ╭──────┴───────╮                     │     
        ╭───────┴───────╮ ╭────┴───╮             ╭───┴──╮  
        │metricsLen eq 1│ │fallback│             │metric│  
        ╰───────┬───────╯ ╰────┬───╯             ╰──────╯  
                │              │                           
             ╭──┴──╮       ╭───┴──╮                        
             │event│       │metric│                        
             ╰─────╯       ╰──────╯                        
topic: spBv1.0/vehicles/DDATA/1234-1234/bus
payload:
{
  "timestamp": "1632139517114",
  "metrics": [
    {
      "name": "Device Control/Scan Rate ms",
      "timestamp": "1632139517114",
      "datatype": 3,
      "intValue": 6000
    },
    {
      "name": "TestMetric1",
      "timestamp": "1632139517114",
      "datatype": 11,
      "booleanValue": false
    }
  ],
  "seq": "0"
}
result: metric
representation:
╭──────╮ 
│metric│ 
╰──────╯ 
```

Basically is there to help you test your decision tree. Notice that at the end you have a result which have a name.
and a representation, if the result is not printed or the representation is not a leave node that means your tree has
some issue, and you need to check it.