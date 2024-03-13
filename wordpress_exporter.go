package main

import (
    "net/http"

    log "github.com/sirupsen/logrus"
    "github.com/prometheus/client_golang/prometheus/promhttp"
    "github.com/prometheus/client_golang/prometheus"

    "flag"
    "fmt"
    "os"
    "io/ioutil"
    "strings"
    "regexp"

    "database/sql"
    _ "github.com/go-sql-driver/mysql"
)

//This is my collector metrics
type wpCollector struct {
    numPostsMetric *prometheus.Desc
    numCommentsMetric *prometheus.Desc
    numUsersMetric *prometheus.Desc

    db_host string
    db_name string
    db_user string
    db_pass string
    db_table_prefix string
}

//This is a constructor for my wpCollector struct
func newWordPressCollector(host string, dbname string, username string, pass string, table_prefix string) *wpCollector {
    return &wpCollector{
        numPostsMetric: prometheus.NewDesc("wp_num_posts_metric",
                        "Shows the number of total posts in the WordPress site",
                        nil, nil,
        ),
        numCommentsMetric: prometheus.NewDesc("wp_num_comments_metric",
                           "Shows the number of total comments in the WordPress site",
                           nil, nil,
        ),
        numUsersMetric: prometheus.NewDesc("wp_num_users_metric",
                        "Shows the number of registered users in the WordPress site",
                        nil, nil,
        ),

        db_host: host,
        db_name: dbname,
        db_user: username,
        db_pass: pass,
        db_table_prefix: table_prefix,
    }
}

//Describe method is required for a prometheus.Collector type
func (collector *wpCollector) Describe(ch chan<- *prometheus.Desc) {

    //We set the metrics
    ch <- collector.numPostsMetric
    ch <- collector.numCommentsMetric
    ch <- collector.numUsersMetric
}

//Collect method is required for a prometheus.Collector type
func (collector *wpCollector) Collect(ch chan<- prometheus.Metric) {

    //We run DB queries here to retrieve the metrics we care about
    dsn := fmt.Sprintf("%s:%s@tcp(%s)/%s", collector.db_user, collector.db_pass, collector.db_host, collector.db_name)

    db, err := sql.Open("mysql", dsn)
    if(err != nil){
        fmt.Fprintf(os.Stderr, "Error connecting to database: %s ...\n", err)
        os.Exit(1)
    }
    defer db.Close()

    //select count(*) as num_users from wp_users;
    var num_users float64
    q1 := fmt.Sprintf("select count(*) as num_users from %susers;", collector.db_table_prefix)
    err = db.QueryRow(q1).Scan(&num_users)
    if err != nil {
        log.Fatal(err)
    }

    //select count(*) as num_comments from wp_comments;
    var num_comments float64
    q2 := fmt.Sprintf("select count(*) as num_comments from %scomments;", collector.db_table_prefix)
    err = db.QueryRow(q2).Scan(&num_comments)
    if err != nil {
        log.Fatal(err)
    }

    //select count(*) as num_posts from wp_posts WHERE post_type='post' AND post_status!='auto-draft';
    var num_posts float64
    q3 := fmt.Sprintf("select count(*) as num_posts from %sposts WHERE post_type='post' AND post_status!='auto-draft';", collector.db_table_prefix)
    err = db.QueryRow(q3).Scan(&num_posts)
    if err != nil {
        log.Fatal(err)
    }

    //Write latest value for each metric in the prometheus metric channel.
    //Note that you can pass CounterValue, GaugeValue, or UntypedValue types here.
    ch <- prometheus.MustNewConstMetric(collector.numPostsMetric, prometheus.CounterValue, num_posts)
    ch <- prometheus.MustNewConstMetric(collector.numCommentsMetric, prometheus.CounterValue, num_comments)
    ch <- prometheus.MustNewConstMetric(collector.numUsersMetric, prometheus.CounterValue, num_users)

}

func main() {

    wpConfPtr := flag.String("wpconfig", "", "Path for wp-config.php file of the WordPress site you wish to monitor")
    wpHostPtr := flag.String("host", "127.0.0.1", "Hostname or Address for DB server")
    wpPortPtr := flag.String("port", "3306", "DB server port")
    wpNamePtr := flag.String("db", "", "DB name")
    wpUserPtr := flag.String("user", "", "DB user for connection")
    wpPassPtr := flag.String("pass", "", "DB password for connection")
    wpTablePrefixPtr := flag.String("tableprefix", "wp_", "Table prefix for WordPress tables")

    flag.Parse()

    if *wpConfPtr == "" {
        db_host := fmt.Sprintf("%s:%s", *wpHostPtr, *wpPortPtr)
        db_name := *wpNamePtr
        db_user := *wpUserPtr
        db_password := *wpPassPtr
        table_prefix := *wpTablePrefixPtr

        if db_name == "" {
            fmt.Fprintf(os.Stderr, "flag -db=dbname required!\n")
            os.Exit(1)
        }

        if db_user == "" {
            fmt.Fprintf(os.Stderr, "flag -user=username required!\n")
            os.Exit(1)
        }

        //We create the collector
        collector := newWordPressCollector(db_host, db_name, db_user, db_password, table_prefix)
        prometheus.MustRegister(collector)

        //no path supplied error
        //fmt.Fprintf(os.Stderr, "flag -wpconfig=/path/to/wp-config/ required!\n")
        //os.Exit(1)
    } else{
        var wpconfig_file strings.Builder
        wpconfig_file.WriteString(*wpConfPtr)

        if strings.HasSuffix(*wpConfPtr, "/") {
            wpconfig_file.WriteString("wp-config.php")
        }else{
            wpconfig_file.WriteString("/wp-config.php")
        }

        //try to read wp-config.php file from path
        dat, err := ioutil.ReadFile(wpconfig_file.String())
        if(err != nil){
            panic(err)
        }
        fmt.Printf("Read :%v bytes\n", len(dat))

        //We must locate with regular expressions the MySQL connection credentials and the table prefix
        //define('DB_HOST', 'xxxxxxx');
        r, _ := regexp.Compile(`define\(['"]DB_HOST['"].*?,.*?['"](.*?)['"].*?\);`)
        res := r.FindStringSubmatch(string(dat[:len(dat)]))
        if(res == nil){
            fmt.Fprintf(os.Stderr, "Error could not find DB_HOST in wp-config.php ...\n")
            os.Exit(1)
        }
        db_host := res[1]
    
        //define('DB_NAME', 'xxxxxxx');
        r, _ = regexp.Compile(`define\(['"]DB_NAME['"].*?,.*?['"](.*?)['"].*?\);`)
        res = r.FindStringSubmatch(string(dat[:len(dat)]))
        if(res == nil){
            fmt.Fprintf(os.Stderr, "Error could not find DB_NAME in wp-config.php ...\n")
            os.Exit(1)
        }
        db_name := res[1]

        //define('DB_USER', 'xxxxxxx');
        r, _ = regexp.Compile(`define\(['"]DB_USER['"].*?,.*?['"](.*?)['"].*?\);`)
        res = r.FindStringSubmatch(string(dat[:len(dat)]))
        if(res == nil){
            fmt.Fprintf(os.Stderr, "Error could not find DB_USER in wp-config.php ...\n")
            os.Exit(1)
        }
        db_user := res[1]

        //define('DB_PASSWORD', 'xxxxxxx');
        r, _ = regexp.Compile(`define\(['"]DB_PASSWORD['"].*?,.*?['"](.*?)['"].*?\);`)
        res = r.FindStringSubmatch(string(dat[:len(dat)]))
        if(res == nil){
            fmt.Fprintf(os.Stderr, "Error could not find DB_PASSWORD in wp-config.php ...\n")
            os.Exit(1)
        }
        db_password := res[1]

        //$table_prefix  = 'wp_';
        r, _ = regexp.Compile(`\$table_prefix.*?=.*?['"](.*?)['"];`)
        res = r.FindStringSubmatch(string(dat[:len(dat)]))
        if(res == nil){
            fmt.Fprintf(os.Stderr, "Error could not find $table_prefix in wp-config.php ...\n")
            os.Exit(1)
        }
        table_prefix := res[1]

        //We create the collector
        collector := newWordPressCollector(db_host, db_name, db_user, db_password, table_prefix)
        prometheus.MustRegister(collector)
    }

    //This section will start the HTTP server and expose
    //any metrics on the /metrics endpoint.
    http.Handle("/metrics", promhttp.Handler())
    log.Info("Beginning to serve on port :8888")
    log.Fatal(http.ListenAndServe(":8888", nil))
}

