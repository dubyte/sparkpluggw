#Decision tree
sparkpluggww support metrics and events to know what is an event and what is an event it uses a decision tree.
To use the decision tree it needs the next two:
- An attribute map (map[string]interface)
- A decision tree

The library with a decision tree loaded receives the attribute map and return the value of the node where 
the decision tree was left after evaluate it.

# The library
The library used for this project is https://github.com/tkanos/go-dtree so all
the documentation there is valid here. The version is v0.5.0

# How does it work
+ Define a tree
+ Get the attributes of the payload
+ Pass the attributes to the tree resolver
+ get the name of the result node.

# The attribute map
sparkpluggw receive mqtt messages in the sparkplug format and each message belongs to a topic. Internally in the package
message there is a function that receives a topic and a sparkplug payload and currently it returns only two attributes:

```go
func Attributes(topic string, payload pb.Payload) map[string]interface{} {
    attr := make(map[string]interface{})
    attr["firstMetricIs"] = firstMetric(payload)
    attr["metricsLen"] = fmt.Sprintf("%d", len(payload.Metrics))
return attr
}
```

so that means that when we receive a mqtt message with send the topic and the payload to get the attributes map that 
describe the message, so we can pass to the decision tree, and it can make decisions base on the attributes.
There are more types supported by the library, but currently we only have string attributes.

## The decision tree
You need a json file which will be a list of objects:
- The root node that has only `id` and `name`.
```json
  {
    "id": 1,
    "name": "root"
  }
```
- Operation nodes (node with id 2) which have an `id`, `parent_id`, `key`, `operator` and `value`. (optional could have an unused name )
```json
  {
    "id": 2,
    "parent_id": 1,
    "key": "firstMetricIs",
    "operator": "eq",
    "value": "Device Control/Scan Rate ms"
  }
```
the operation node is the one that is takes the attribute map and will compare the key `firstMetricIs` with the attribute map
and if it is equal (`eq`) to `Device Control/Scan Rate ms` it will evaluate true and it will continue in this path.

- A leaf node (like node with id 4) which have `id`, `parent_id`, `name`
```json
  {
    "id": 4,
    "parent_id": 2,
    "Name": "metric"
  }
```
This is like an exit node if the decision tree ends in this node it returns `name` and in sparkpluggw it is used to 
classify the message as `metric`

- A fallback node `id`, `parent_id`, `value`
```json
  {
    "id": 3,
    "parent_id": 1,
    "value": "fallback"
  }
```
This fallback works like and "else" if the condition in the other nodes does not evaluate true it will go in this path.

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

## Examples

### Use name in operand nodes
In the next example if the operand have a name the printed representation only show the name. So you could be used
to make user user-friendly annotations instead of operations.

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

### Fallback nodes
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

### A decision tree and a payload example to show the result
The next example use firstMetricIs and metricsLen to make a decision tree 
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

## Commands
### decisiontreeplay
decision tree play is a code in sparkploggw that load a decision tree and has a event example payload and a metric payload
running the command allow you to see your decision tree and the payload in consideration and it will print the outcome

In the next example the tree ends in `metric` using the next topic and payload 
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

### treeshow
treeshow it only receives a json as parameter and if all is ok it will print three representation.